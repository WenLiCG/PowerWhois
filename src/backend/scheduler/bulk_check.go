package scheduler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"

	"github.com/redis/go-redis/v9"
)

const (
	MaxUploadBytes = 100 << 20
	maxBulkDomains = 1_000_000
	pageSize       = 256
	statusRequeue  = "requeuing"

	statusKey  = constant.BulkCheckStatusRedisKey
	modeKey    = constant.BulkCheckQueryTypeRedisKey
	rawKey     = constant.BulkCheckRawDomainsRedisKey
	pendingKey = constant.BulkCheckUniqueDomainsRedisKey
	totalKey   = constant.BulkCheckUniqueDomainsCountRedisKey
	takenKey   = constant.BulkCheckTakenResultRedisKey
	freeKey    = constant.BulkCheckFreeResultRedisKey
	errorKey   = constant.BulkCheckErrorResultRedisKey
	uploadKey  = rawKey + ":upload"
)

var ErrBulkState = errors.New("当前批量任务状态不允许此操作")
var ErrUploadSize = errors.New("批量文件必须非空且至多 100 MiB")

type BulkCheck struct {
	redis *redis.Client
	// A single process owns the legacy single-task keys and serializes administrative actions.
	mu      sync.Mutex
	active  *bulkRun
	failure string
	closing bool

	query func(context.Context, string, string, bool, config.Config) lookupinfo.QueryResult
}

type bulkRun struct {
	cancel     context.CancelFunc
	done       chan struct{}
	stopStatus string
}

type BulkCheckDomain struct {
	Domain string
	Order  int
}

type BulkCheckStatusInfo struct {
	Status        string
	QueryType     string
	TotalDomains  int64
	RemainDomains int64
	TakenDomains  int64
	FreeDomains   int64
	ErrorDomains  int64
	Error         string `json:",omitempty"`
	CanResume     bool   `json:",omitempty"`
}

func NewBulkCheck(ctx context.Context, client *redis.Client) (*BulkCheck, error) {
	b := &BulkCheck{redis: client, query: lookuper.Query}
	status, err := b.status(ctx)
	if err != nil {
		return nil, err
	}
	switch status {
	case constant.BulkCheckStatusUniquing, constant.BulkCheckStatusRunning, statusRequeue:
		if err := b.resume(ctx, status); err != nil {
			b.failure = "无法恢复批量任务，请检查查询接口配置后重试"
		}
	}
	return b, nil
}

func (b *BulkCheck) status(ctx context.Context) (string, error) {
	s, err := b.redis.Get(ctx, statusKey).Result()
	if errors.Is(err, redis.Nil) {
		return constant.BulkCheckStatusIdle, nil
	}
	return s, err
}

func (b *BulkCheck) Info(ctx context.Context) (BulkCheckStatusInfo, error) {
	var failure string
	canResume := false
	if b.mu.TryLock() {
		failure = b.failure
		canResume = b.active == nil && !b.closing
		b.mu.Unlock()
	}
	p := b.redis.TxPipeline()
	status := p.Get(ctx, statusKey)
	mode := p.Get(ctx, modeKey)
	total := p.Get(ctx, totalKey)
	pending := p.HLen(ctx, pendingKey)
	taken := p.LLen(ctx, takenKey)
	free := p.LLen(ctx, freeKey)
	failed := p.LLen(ctx, errorKey)
	_, err := p.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return BulkCheckStatusInfo{}, err
	}
	for _, cmd := range []redis.Cmder{status, mode, total, pending, taken, free, failed} {
		if err := cmd.Err(); err != nil && !errors.Is(err, redis.Nil) {
			return BulkCheckStatusInfo{}, err
		}
	}
	n, err := total.Int64()
	if err != nil && !errors.Is(err, redis.Nil) {
		return BulkCheckStatusInfo{}, err
	}
	s := status.Val()
	canResume = canResume && (s == constant.BulkCheckStatusPaused || s == constant.BulkCheckStatusRunning || s == statusRequeue || s == constant.BulkCheckStatusUniquing)
	switch {
	case failure != "":
		s = constant.BulkCheckStatusError
	case s == statusRequeue:
		s = constant.BulkCheckStatusUniquing
	case s == "":
		s = constant.BulkCheckStatusIdle
	}
	return BulkCheckStatusInfo{
		Status: s, QueryType: mode.Val(), TotalDomains: n, RemainDomains: pending.Val(),
		TakenDomains: taken.Val(), FreeDomains: free.Val(), ErrorDomains: failed.Val(), Error: failure, CanResume: canResume,
	}, nil
}

func (b *BulkCheck) Upload(ctx context.Context, input io.Reader) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil || b.closing {
		return ErrBulkState
	}
	if s, err := b.status(ctx); err != nil {
		return err
	} else if s != constant.BulkCheckStatusIdle && s != constant.BulkCheckStatusInit {
		return ErrBulkState
	}
	if err := b.redis.Set(ctx, uploadKey, "", time.Hour).Err(); err != nil {
		return err
	}
	defer b.redis.Del(context.WithoutCancel(ctx), uploadKey)

	reader := io.LimitReader(input, MaxUploadBytes+1)
	buf := make([]byte, 256<<10)
	var offset int64
	for {
		n, err := io.ReadFull(reader, buf)
		if offset+int64(n) > MaxUploadBytes {
			return ErrUploadSize
		}
		if n != 0 {
			if err := b.redis.SetRange(ctx, uploadKey, offset, string(buf[:n])).Err(); err != nil {
				return err
			}
			offset += int64(n)
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if offset == 0 {
		return ErrUploadSize
	}

	// Publish input and status together. SetRange also makes retries of a chunk idempotent.
	if err := publishUpload.Run(ctx, b.redis, []string{uploadKey, rawKey, statusKey}, constant.BulkCheckStatusInit).Err(); err != nil {
		return err
	}
	b.failure = ""
	return nil
}

var publishUpload = redis.NewScript(`
redis.call('RENAME', KEYS[1], KEYS[2])
redis.call('PERSIST', KEYS[2])
redis.call('SET', KEYS[3], ARGV[1])
return 1
`)

func (b *BulkCheck) Start(ctx context.Context, mode string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil || b.closing || !lookuper.ValidMode(mode, true, config.GetConfig()) {
		return ErrBulkState
	}
	s, err := b.status(ctx)
	if err != nil {
		return err
	}
	if s != constant.BulkCheckStatusInit {
		return ErrBulkState
	}
	_, err = b.redis.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, modeKey, mode, 0)
		p.Set(ctx, statusKey, constant.BulkCheckStatusUniquing, 0)
		return nil
	})
	if err == nil {
		b.launch(constant.BulkCheckStatusUniquing, mode)
	}
	return err
}

func (b *BulkCheck) Resume(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil || b.closing {
		return ErrBulkState
	}
	s, err := b.status(ctx)
	if err != nil {
		return err
	}
	switch s {
	case constant.BulkCheckStatusPaused, constant.BulkCheckStatusRunning, constant.BulkCheckStatusUniquing, statusRequeue:
		return b.resume(ctx, s)
	default:
		return ErrBulkState
	}
}

func (b *BulkCheck) resume(ctx context.Context, stage string) error {
	mode, err := b.redis.Get(ctx, modeKey).Result()
	if err != nil {
		return err
	}
	if !lookuper.ValidMode(mode, true, config.GetConfig()) {
		return errors.New("批量任务的查询接口当前不可用")
	}
	b.launch(stage, mode)
	return nil
}

func (b *BulkCheck) Recheck(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil || b.closing {
		return ErrBulkState
	}
	status, err := b.status(ctx)
	if err != nil {
		return err
	}
	if status != constant.BulkCheckStatusDone {
		return ErrBulkState
	}
	mode, err := b.redis.Get(ctx, modeKey).Result()
	if err != nil {
		return err
	}
	if !lookuper.ValidMode(mode, true, config.GetConfig()) {
		return errors.New("批量任务的查询接口当前不可用")
	}
	if err := b.redis.Set(ctx, statusKey, statusRequeue, 0).Err(); err != nil {
		return err
	}
	b.launch(statusRequeue, mode)
	return nil
}

func (b *BulkCheck) Stop(ctx context.Context, target string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, err := b.status(ctx)
	if err != nil {
		return err
	}
	if b.active == nil {
		if target != constant.BulkCheckStatusCanceled || (s != constant.BulkCheckStatusInit && s != constant.BulkCheckStatusPaused) {
			return ErrBulkState
		}
		return b.redis.Set(ctx, statusKey, target, 0).Err()
	}
	if target == constant.BulkCheckStatusPaused && s != constant.BulkCheckStatusRunning {
		return ErrBulkState
	}
	b.active.stopStatus = target
	b.active.cancel()
	return nil
}

func (b *BulkCheck) Clear(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil || b.closing {
		return errors.New("请先暂停或取消并等待批量任务停止")
	}
	if err := b.redis.Del(ctx, statusKey, modeKey, uploadKey, rawKey, pendingKey, totalKey, takenKey, freeKey, errorKey).Err(); err != nil {
		return err
	}
	b.failure = ""
	return nil
}

func (b *BulkCheck) Shutdown(ctx context.Context) error {
	for !b.mu.TryLock() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
	b.closing = true
	run := b.active
	if run != nil {
		run.stopStatus = ""
		run.cancel()
	}
	b.mu.Unlock()
	if run == nil {
		return nil
	}
	select {
	case <-run.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *BulkCheck) launch(stage, mode string) {
	ctx, cancel := context.WithCancel(context.Background())
	run := &bulkRun{cancel: cancel, done: make(chan struct{})}
	b.active, b.failure = run, ""
	go func() {
		err := b.run(ctx, stage, mode)
		b.mu.Lock()
		defer b.mu.Unlock()
		defer close(run.done)
		b.active = nil
		defer cancel()

		var status string
		switch {
		case run.stopStatus != "":
			status = run.stopStatus
		case b.closing:
			return // Running and preparation stages can be resumed from Redis.
		case err == nil:
			status = constant.BulkCheckStatusDone
		case errors.Is(err, errBulkInput):
			status = constant.BulkCheckStatusError
			b.failure = err.Error()
		default:
			b.failure = "批量任务中断，已保存结果可下载，剩余域名可继续"
			log.Error("Bulk task interrupted: ", err)
			return // Retain the saved stage so a restart or Resume can retry it.
		}
		saveCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if err := b.redis.Set(saveCtx, statusKey, status, 0).Err(); err != nil {
			b.failure = "无法保存批量任务状态，请确认 Redis 连接后重试"
			log.Error("Could not save bulk task state: ", err)
		}
	}()
}

var errBulkInput = errors.New("批量输入无有效域名或超出文件限制")

type rawReader struct {
	ctx    context.Context
	redis  *redis.Client
	offset int64
}

func (r *rawReader) Read(buf []byte) (int, error) {
	data, err := r.redis.GetRange(r.ctx, rawKey, r.offset, r.offset+int64(len(buf))-1).Result()
	if err != nil {
		return 0, err
	}
	if data == "" {
		return 0, io.EOF
	}
	r.offset += int64(len(data))
	return copy(buf, data), nil
}

func (b *BulkCheck) prepare(ctx context.Context) (int64, error) {
	if err := b.redis.Del(ctx, pendingKey, totalKey, takenKey, freeKey, errorKey).Err(); err != nil {
		return 0, err
	}
	rows := bufio.NewScanner(bufio.NewReaderSize(&rawReader{ctx: ctx, redis: b.redis}, 256<<10))
	rows.Buffer(make([]byte, 4096), 64<<10)
	p := b.redis.Pipeline()
	queued := 0
	for line := 0; rows.Scan(); line++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if line >= maxBulkDomains {
			return 0, errBulkInput
		}
		input := rows.Text()
		if line == 0 {
			input = strings.TrimPrefix(input, "\uFEFF")
		}
		name, err := utils.TrimAndGetMainDomain(input)
		if err != nil {
			continue
		}
		raw, _ := json.Marshal(BulkCheckDomain{Domain: name, Order: line})
		p.HSetNX(ctx, pendingKey, name, raw)
		queued++
		if queued == pageSize {
			if _, err := p.Exec(ctx); err != nil {
				return 0, err
			}
			queued = 0
		}
	}
	if err := rows.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return 0, errBulkInput
		}
		return 0, err
	}
	if queued != 0 {
		if _, err := p.Exec(ctx); err != nil {
			return 0, err
		}
	}
	total, err := b.redis.HLen(ctx, pendingKey).Result()
	if err == nil && total == 0 {
		return 0, errBulkInput
	}
	return total, err
}

var moveErrors = redis.NewScript(`
local rows = redis.call('LRANGE', KEYS[1], 0, tonumber(ARGV[1])-1)
for _, raw in ipairs(rows) do
	local row = cjson.decode(raw)
	if type(row.domain) ~= 'string' or row.domain == '' or type(row.order) ~= 'number' then
		return redis.error_reply('invalid saved result')
	end
end
for _, raw in ipairs(rows) do
	local row = cjson.decode(raw)
	redis.call('HSETNX', KEYS[2], row.domain, cjson.encode({Domain=row.domain,Order=row.order}))
	redis.call('LPOP', KEYS[1])
end
return #rows
`)

func (b *BulkCheck) run(ctx context.Context, stage, mode string) error {
	switch stage {
	case constant.BulkCheckStatusUniquing:
		total, err := b.prepare(ctx)
		if err != nil {
			return err
		}
		if err := b.redis.Set(ctx, totalKey, total, 0).Err(); err != nil {
			return err
		}
	case statusRequeue:
		for {
			moved, err := moveErrors.Run(ctx, b.redis, []string{errorKey, pendingKey}, pageSize).Int()
			if err != nil {
				return err
			}
			if moved == 0 {
				break
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.redis.Set(ctx, statusKey, constant.BulkCheckStatusRunning, 0).Err(); err != nil {
		return err
	}
	cfg := config.GetConfig()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan BulkCheckDomain, cfg.BulkCheckConcurrencyLimit)
	firstErr := make(chan error, 1)
	var workers sync.WaitGroup
	for range cfg.BulkCheckConcurrencyLimit {
		workers.Go(func() {
			for item := range work {
				if ctx.Err() != nil {
					return
				}
				result := b.query(ctx, item.Domain, mode, true, cfg)
				if ctx.Err() != nil {
					return
				}
				result.Order = item.Order
				if err := b.commit(ctx, result); err != nil {
					select {
					case firstErr <- err:
					default:
					}
					cancel()
					return
				}
			}
		})
	}
	err := b.dispatch(ctx, work)
	if err != nil {
		cancel()
	}
	close(work)
	workers.Wait()
	select {
	case workerErr := <-firstErr:
		return workerErr
	default:
	}
	if err != nil {
		return err
	}
	remaining, err := b.redis.HLen(ctx, pendingKey).Result()
	if err == nil && remaining != 0 {
		return errors.New("redis scan did not finish all pending items")
	}
	return err
}

func (b *BulkCheck) dispatch(ctx context.Context, work chan<- BulkCheckDomain) error {
	var cursor uint64
	for {
		fields, next, err := b.redis.HScan(ctx, pendingKey, cursor, "", pageSize).Result()
		if err != nil {
			return err
		}
		for i := 0; i+1 < len(fields); i += 2 {
			var item BulkCheckDomain
			if err := json.Unmarshal([]byte(fields[i+1]), &item); err != nil || item.Domain != fields[i] || item.Order < 0 {
				return errors.New("invalid saved pending item")
			}
			select {
			case work <- item:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

var saveResult = redis.NewScript(`
if redis.call('HEXISTS', KEYS[1], ARGV[1]) == 0 then return 0 end
redis.call('RPUSH', KEYS[2], ARGV[2])
redis.call('HDEL', KEYS[1], ARGV[1])
return 1
`)

func (b *BulkCheck) commit(ctx context.Context, result lookupinfo.QueryResult) error {
	key := errorKey
	switch result.RegisterStatus {
	case constant.DomainRegisterStatusTaken:
		key = takenKey
	case constant.DomainRegisterStatusFree:
		key = freeKey
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return saveResult.Run(ctx, b.redis, []string{pendingKey, key}, result.Domain, raw).Err()
}
