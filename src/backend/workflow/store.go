package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var ErrNotFound = errors.New("job not found")
var ErrConflict = errors.New("job state conflict")
var jobsKey = []byte("jobs")
var metaKey = []byte("meta")

type Store struct {
	db           *bolt.DB
	beforeCommit func() error
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func validID(id string) bool { b, err := hex.DecodeString(id); return err == nil && len(b) == 16 }
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	err = s.update(func(tx *bolt.Tx) error {
		jobs, err := tx.CreateBucketIfNotExists(jobsKey)
		if err != nil {
			return err
		}
		return jobs.ForEach(func(k, v []byte) error {
			if v != nil {
				return nil
			}
			b := jobs.Bucket(k)
			j, err := readJob(b)
			if err != nil {
				return err
			}
			pending := b.Bucket([]byte("pending"))
			active := b.Bucket([]byte("active"))
			if err = active.ForEach(func(k, v []byte) error { return pending.Put(k, []byte{1}) }); err != nil {
				return err
			}
			if err = b.DeleteBucket([]byte("active")); err != nil {
				return err
			}
			if _, err = b.CreateBucket([]byte("active")); err != nil {
				return err
			}
			if j.State == "running" {
				j.State = "paused"
				j.Error = "interrupted process; pending and in-flight domains recovered; resume explicitly"
			}
			if j.State == "importing" {
				j.State = "error"
				j.Error = "interrupted import; re-import the source workbook"
			}
			return putJob(b, j)
		})
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) update(fn func(*bolt.Tx) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		if s.beforeCommit != nil {
			return s.beforeCommit()
		}
		return nil
	})
}
func readJob(b *bolt.Bucket) (Job, error) {
	var j Job
	if b == nil {
		return j, ErrNotFound
	}
	err := json.Unmarshal(b.Get(metaKey), &j)
	j.Partial = j.Completed != j.Candidates || j.State != "completed"
	return j, err
}
func putJob(b *bolt.Bucket, j Job) error {
	j.UpdatedAt = time.Now().UTC()
	j.Partial = j.Completed != j.Candidates || j.State != "completed"
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return b.Put(metaKey, data)
}
func jobBucket(tx *bolt.Tx, id string) (*bolt.Bucket, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	b := tx.Bucket(jobsKey).Bucket([]byte(id))
	if b == nil {
		return nil, ErrNotFound
	}
	return b, nil
}
func rowKey(n int) []byte { return []byte(fmt.Sprintf("%09d", n)) }

func (s *Store) Get(id string) (Job, error) {
	var j Job
	err := s.db.View(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, id)
		if e != nil {
			return e
		}
		j, e = readJob(b)
		return e
	})
	return j, err
}
func (s *Store) List(after string, limit int) ([]Job, error) {
	var out = []Job{}
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(jobsKey).Cursor()
		k, v := c.First()
		if after != "" {
			k, v = c.Seek([]byte(after))
			if string(k) == after {
				k, v = c.Next()
			}
		}
		for ; k != nil && len(out) < limit; k, v = c.Next() {
			if v != nil {
				continue
			}
			j, e := readJob(tx.Bucket(jobsKey).Bucket(k))
			if e != nil {
				return e
			}
			out = append(out, j)
		}
		return nil
	})
	return out, err
}

func (s *Store) Page(id, kind, after string, limit int) ([]json.RawMessage, string, error) {
	items := []json.RawMessage{}
	next := ""
	err := s.db.View(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, id)
		if e != nil {
			return e
		}
		if kind != "items" && kind != "seeds" && kind != "origins" {
			return fmt.Errorf("invalid page kind")
		}
		c := b.Bucket([]byte(kind)).Cursor()
		k, v := c.First()
		if after != "" {
			k, v = c.Seek([]byte(after))
			if string(k) == after {
				k, v = c.Next()
			}
		}
		for ; k != nil && len(items) < limit; k, v = c.Next() {
			items = append(items, append(json.RawMessage(nil), v...))
			next = string(k)
		}
		if k == nil {
			next = ""
		}
		return nil
	})
	return items, next, err
}

func (s *Store) Transition(id, action string) (Job, error) {
	var j Job
	err := s.update(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, id)
		if e != nil {
			return e
		}
		j, e = readJob(b)
		if e != nil {
			return e
		}
		switch action {
		case "start", "resume":
			if j.State == "running" {
				return nil
			}
			if j.State != "ready" && j.State != "paused" {
				return fmt.Errorf("%w: cannot start %s", ErrConflict, j.State)
			}
			j.State = "running"
			j.Error = ""
		case "pause":
			if j.State == "paused" {
				return nil
			}
			if j.State != "running" {
				return fmt.Errorf("%w: cannot pause %s", ErrConflict, j.State)
			}
			j.State = "paused"
		case "cancel":
			if j.State == "canceled" {
				return nil
			}
			if j.State == "generating" || j.State == "importing" || j.State == "completed" {
				return fmt.Errorf("%w: cannot cancel %s", ErrConflict, j.State)
			}
			j.State = "canceled"
		default:
			return fmt.Errorf("unknown action")
		}
		if j.State != "running" {
			active := b.Bucket([]byte("active"))
			pending := b.Bucket([]byte("pending"))
			if e = active.ForEach(func(k, v []byte) error { return pending.Put(k, []byte{1}) }); e != nil {
				return e
			}
			if e = b.DeleteBucket([]byte("active")); e != nil {
				return e
			}
			if _, e = b.CreateBucket([]byte("active")); e != nil {
				return e
			}
		}
		return putJob(b, j)
	})
	return j, err
}

type claim struct{ JobID, Domain, Lease string }

func (s *Store) claim() (*claim, error) {
	var work *claim
	// Avoid a write transaction and fsync on every idle worker poll.
	hasWork := false
	if err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(jobsKey).ForEach(func(k, v []byte) error {
			if v != nil {
				return nil
			}
			b := tx.Bucket(jobsKey).Bucket(k)
			j, e := readJob(b)
			if e != nil {
				return e
			}
			if j.State == "running" {
				hasWork = true
			}
			return nil
		})
	}); err != nil {
		return nil, err
	}
	if !hasWork {
		return nil, nil
	}
	err := s.update(func(tx *bolt.Tx) error {
		jobs := tx.Bucket(jobsKey)
		c := jobs.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if v != nil {
				continue
			}
			b := jobs.Bucket(k)
			j, e := readJob(b)
			if e != nil {
				return e
			}
			if j.State != "running" {
				continue
			}
			p := b.Bucket([]byte("pending"))
			domain, _ := p.Cursor().First()
			if domain == nil {
				if b.Bucket([]byte("active")).Stats().KeyN == 0 {
					j.State = "completed"
					if e = putJob(b, j); e != nil {
						return e
					}
				}
				continue
			}
			d := string(domain)
			lease := newID()
			if e = b.Bucket([]byte("active")).Put([]byte(d), []byte(lease)); e != nil {
				return e
			}
			if e = p.Delete([]byte(d)); e != nil {
				return e
			}
			work = &claim{string(k), d, lease}
			return nil
		}
		return nil
	})
	return work, err
}

func (s *Store) finish(work claim, result Result) error {
	return s.update(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, work.JobID)
		if e != nil {
			return e
		}
		active := b.Bucket([]byte("active"))
		if string(active.Get([]byte(work.Domain))) != work.Lease {
			return nil
		}
		j, e := readJob(b)
		if e != nil {
			return e
		}
		if j.State != "running" {
			return nil
		}
		items := b.Bucket([]byte("items"))
		var item Candidate
		if e = json.Unmarshal(items.Get([]byte(work.Domain)), &item); e != nil {
			return e
		}
		if item.Result != nil {
			return fmt.Errorf("result already committed")
		}
		item.Result = &result
		data, e := json.Marshal(item)
		if e != nil {
			return e
		}
		if e = items.Put([]byte(work.Domain), data); e != nil {
			return e
		}
		if e = active.Delete([]byte(work.Domain)); e != nil {
			return e
		}
		j.Completed++
		j.Counts[result.Status]++
		if j.Completed == j.Candidates {
			j.State = "completed"
		}
		return putJob(b, j)
	})
}

func (s *Store) stats(id string) (pending, active int, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		b, e := jobBucket(tx, id)
		if e != nil {
			return e
		}
		pending = b.Bucket([]byte("pending")).Stats().KeyN
		active = b.Bucket([]byte("active")).Stats().KeyN
		return nil
	})
	return
}
func keyParts(k string) (string, int, string) {
	parts := strings.Split(k, "\x00")
	n, _ := strconv.Atoi(parts[1])
	return parts[0], n, parts[2]
}
