local raw = redis.call("GET", KEYS[1])
if not raw then
  return nil
end

local cache = cjson.decode(raw)
local quizzes = cache["quizzes"] or {}

if #quizzes == 0 then
  return nil
end

local current_id = cache["current_quiz_id"]
if current_id and current_id ~= "" then
  for _, quiz in ipairs(quizzes) do
    if quiz["id"] == current_id then
      return cjson.encode({ found = true, quiz = quiz, remaining = #quizzes })
    end
  end
end

local index = math.random(#quizzes)
local quiz = quizzes[index]
cache["current_quiz_id"] = quiz["id"]
redis.call("SET", KEYS[1], cjson.encode(cache), "EX", ARGV[1])

return cjson.encode({ found = true, quiz = quiz, remaining = #quizzes })
