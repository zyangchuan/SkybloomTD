local raw = redis.call("GET", KEYS[1])
if not raw then
  return nil
end

local cache = cjson.decode(raw)
local appended = cjson.decode(ARGV[1])

if cache["quizzes"] == nil then
  cache["quizzes"] = {}
end

for _, quiz in ipairs(appended) do
  table.insert(cache["quizzes"], quiz)
end

redis.call("SET", KEYS[1], cjson.encode(cache), "EX", ARGV[2])

return #cache["quizzes"]
