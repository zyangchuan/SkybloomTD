local raw = redis.call("GET", KEYS[1])
if not raw then
  return nil
end

local cache = cjson.decode(raw)
local quizzes = cache["quizzes"] or {}

for index, quiz in ipairs(quizzes) do
  if quiz["id"] == ARGV[1] then
    table.remove(quizzes, index)
    cache["quizzes"] = quizzes

    if cache["current_quiz_id"] == ARGV[1] then
      cache["current_quiz_id"] = ""
    end

    redis.call("SET", KEYS[1], cjson.encode(cache), "EX", ARGV[2])

    return cjson.encode({ found = true, quiz = quiz, remaining = #quizzes })
  end
end

return cjson.encode({ found = false, remaining = #quizzes })
