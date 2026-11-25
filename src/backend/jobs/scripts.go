package jobs

// Keep large evidence in Redis until it fits the reply. Missing slots retain their indices.
const resultChunk = `
local rows, bytes = {}, 0
local budget, maxRecord = tonumber(ARGV[3]), tonumber(ARGV[4])
for index=tonumber(ARGV[1]),tonumber(ARGV[2])-1 do
 local raw=redis.call('HGET',KEYS[1],index)
 local size=raw and #raw or 0
 if bytes>0 and bytes+size>budget then break end
 if maxRecord>0 and size>maxRecord then
  rows[#rows+1]=size
  break
 end
 rows[#rows+1]=raw
 bytes=bytes+size
end
return rows
`

// Every published task mutation checks ownership in Redis, not only in the local mutex.
const writeGuard = `
local owner=redis.call('GET',KEYS[1])
if (ARGV[1]=='' and owner) or (ARGV[1]~='' and owner~=ARGV[1]) then return redis.error_reply('worker lease lost') end
local function check(key, expected)
 local kind=redis.call('TYPE',key).ok
 if kind~='none' and kind~=expected then error('invalid task storage type') end
end
`

const createScript = writeGuard + `
check(KEYS[2],'string'); check(KEYS[3],'list'); check(KEYS[4],'list'); check(KEYS[5],'zset'); check(KEYS[6],'string')
if redis.call('EXISTS',KEYS[2])~=0 or redis.call('LLEN',KEYS[3])~=tonumber(ARGV[5]) then return redis.error_reply('incomplete task input') end
if ARGV[6]=='1' and redis.call('EXISTS',KEYS[6])~=0 then return redis.error_reply('idempotency publication conflict') end
local commands={{'SET',KEYS[2],ARGV[3]},{'LPUSH',KEYS[4],ARGV[2]}}
if ARGV[6]=='1' then table.insert(commands,{'SET',KEYS[6],ARGV[4]}) end
table.insert(commands,{'ZADD',KEYS[5],ARGV[7],ARGV[2]})
for _,command in ipairs(commands) do
 local result=redis.pcall(unpack(command))
 if type(result)=='table' and result.err then
  redis.pcall('DEL',KEYS[2]); redis.pcall('LREM',KEYS[4],0,ARGV[2]); redis.pcall('ZREM',KEYS[5],ARGV[2])
  if ARGV[6]=='1' then redis.pcall('DEL',KEYS[6]) end
  return redis.error_reply('task publication failed')
 end
end
redis.call('PERSIST',KEYS[3])
return 1
`

const saveScript = writeGuard + `
check(KEYS[2],'string'); check(KEYS[3],'zset')
local result=redis.pcall('SET',KEYS[2],ARGV[2])
if type(result)=='table' and result.err then return redis.error_reply('cannot save task') end
if ARGV[3]=='queued' then return redis.call('ZADD',KEYS[3],ARGV[5],ARGV[4]) end
return redis.call('ZREM',KEYS[3],ARGV[4])
`

const resultScript = writeGuard + `
check(KEYS[2],'hash'); check(KEYS[3],'hash')
if redis.call('HEXISTS',KEYS[2],ARGV[2])==1 then return 0 end
local count=redis.call('HGET',KEYS[3],ARGV[4])
if count and (not tonumber(count) or tonumber(count)<0 or tonumber(count)>=1000000) then return redis.error_reply('invalid task count') end
local result=redis.pcall('HSET',KEYS[2],ARGV[2],ARGV[3])
if type(result)=='table' and result.err then return redis.error_reply('cannot save result') end
result=redis.pcall('HINCRBY',KEYS[3],ARGV[4],1)
if type(result)=='table' and result.err then redis.call('HDEL',KEYS[2],ARGV[2]); return redis.error_reply('cannot save result count') end
return 1
`
