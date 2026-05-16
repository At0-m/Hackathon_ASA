#include "analyzer.h"

#include <algorithm>
#include <cctype>
#include <regex>
#include <sstream>
#include <string>
#include <vector>

namespace aca {
namespace {

struct Step {
  std::string prompt;
  std::string output;
};

struct Dimension {
  std::string name;
  int score;
  int max_score;
  std::string reason;
};

std::string EscapeJson(const std::string& s) {
  std::string out;
  out.reserve(s.size() + 16);
  for (char c : s) {
    switch (c) {
      case '\\': out += "\\\\"; break;
      case '"': out += "\\\""; break;
      case '\n': out += "\\n"; break;
      case '\r': out += "\\r"; break;
      case '\t': out += "\\t"; break;
      default:
        if (static_cast<unsigned char>(c) < 0x20) {
          out += ' ';
        } else {
          out += c;
        }
    }
  }
  return out;
}

std::string UnescapeJsonString(std::string s) {
  std::string out;
  out.reserve(s.size());
  for (size_t i = 0; i < s.size(); ++i) {
    if (s[i] == '\\' && i + 1 < s.size()) {
      char n = s[++i];
      switch (n) {
        case 'n': out += '\n'; break;
        case 'r': out += '\r'; break;
        case 't': out += '\t'; break;
        case '\\': out += '\\'; break;
        case '"': out += '"'; break;
        default: out += n; break;
      }
    } else {
      out += s[i];
    }
  }
  return out;
}

std::string LowerAscii(std::string s) {
  std::transform(s.begin(), s.end(), s.begin(), [](unsigned char c) {
    return static_cast<char>(std::tolower(c));
  });
  return s;
}

bool HasAny(const std::string& s, std::initializer_list<const char*> words) {
  const std::string low = LowerAscii(s);
  for (const char* w : words) {
    if (low.find(w) != std::string::npos) return true;
  }
  return false;
}

int Clamp(int v, int lo, int hi) {
  return std::max(lo, std::min(v, hi));
}

std::vector<std::string> ExtractRepeatedStrings(const std::string& body, const std::string& key) {
  std::vector<std::string> out;
  std::regex re("\\\"" + key + "\\\"\\s*:\\s*\\\"((?:\\\\.|[^\\\"\\\\])*)\\\"");
  auto begin = std::sregex_iterator(body.begin(), body.end(), re);
  auto end = std::sregex_iterator();
  for (auto it = begin; it != end; ++it) out.push_back(UnescapeJsonString((*it)[1].str()));
  return out;
}

std::string ExtractString(const std::string& body, const std::string& key) {
  auto values = ExtractRepeatedStrings(body, key);
  if (values.empty()) return "";
  return values.front();
}

std::vector<Step> ExtractSteps(const std::string& body) {
  auto prompts = ExtractRepeatedStrings(body, "prompt");
  auto outputs = ExtractRepeatedStrings(body, "model_output");
  std::vector<Step> steps;
  for (size_t i = 0; i < prompts.size(); ++i) {
    Step s;
    s.prompt = prompts[i];
    if (i < outputs.size()) s.output = outputs[i];
    steps.push_back(std::move(s));
  }
  return steps;
}

bool HasLabel(const std::vector<Step>& steps, const std::string& label) {
  for (const Step& step : steps) {
    const std::string text = step.prompt + " " + step.output;
    if (label == "clarification" && HasAny(text, {"огранич", "критер", "цель", "формат", "требован"})) return true;
    if (label == "planning" && HasAny(text, {"разбей", "план", "этап", "архитект", "структур", "шаг"})) return true;
    if (label == "context" && HasAny(text, {"контекст", "роль", "вход", "выход", "формат"})) return true;
    if (label == "iteration" && HasAny(text, {"улучш", "исправ", "перепиши", "доработ", "рефактор"})) return true;
    if (label == "verification" && HasAny(text, {"проверь", "ошиб", "edge", "test", "тест", "валид", "галлюцин", "риск"})) return true;
    if (label == "ethics" && HasAny(text, {"bias", "privacy", "этик", "приват", "личн", "предвз"})) return true;
    if (label == "implementation" && HasAny(text, {"код", "сервис", "api", "endpoint", "файл", "реализ"})) return true;
  }
  return false;
}

bool HasLongPrompt(const std::vector<Step>& steps) {
  for (const Step& s : steps) if (s.prompt.size() > 220) return true;
  return false;
}

Dimension MakeDim(const std::string& name, int score, int max_score, const std::string& reason) {
  return Dimension{name, Clamp(score, 0, max_score), max_score, reason};
}

std::string DimJson(const Dimension& d) {
  std::ostringstream os;
  os << "{\"name\":\"" << EscapeJson(d.name) << "\",\"score\":" << d.score
     << ",\"max_score\":" << d.max_score << ",\"reason\":\"" << EscapeJson(d.reason) << "\"}";
  return os.str();
}

std::string StringArrayJson(const std::vector<std::string>& values) {
  std::ostringstream os;
  os << "[";
  for (size_t i = 0; i < values.size(); ++i) {
    if (i) os << ",";
    os << "\"" << EscapeJson(values[i]) << "\"";
  }
  os << "]";
  return os.str();
}

std::string PromptLabelsJson(const std::vector<Step>& steps) {
  std::ostringstream os;
  os << "[";
  for (size_t i = 0; i < steps.size(); ++i) {
    std::vector<std::string> labels;
    const std::vector<Step> one{steps[i]};
    if (HasLabel(one, "clarification")) labels.push_back("clarification");
    if (HasLabel(one, "planning")) labels.push_back("planning");
    if (HasLabel(one, "context")) labels.push_back("context_setting");
    if (HasLabel(one, "implementation")) labels.push_back("implementation");
    if (HasLabel(one, "iteration")) labels.push_back("iteration");
    if (HasLabel(one, "verification")) labels.push_back("verification");
    if (HasLabel(one, "ethics")) labels.push_back("ethics_privacy_bias");
    if (labels.empty()) labels.push_back("unclassified");
    if (i) os << ",";
    os << "{\"step_number\":" << (i + 1) << ",\"labels\":" << StringArrayJson(labels)
       << ",\"notes\":\"Промпт классифицирован по видимым признакам процесса.\"}";
  }
  os << "]";
  return os.str();
}

int ApplyCaps(int total, const std::vector<Step>& steps, const std::string& final_answer, const std::string& self_review, bool has_code) {
  if (steps.empty()) return std::min(total, 25);
  if (steps.size() == 1) total = std::min(total, 45);
  if (steps.size() == 2) total = std::min(total, 60);
  if (!HasLabel(steps, "verification")) total = std::min(total, 66);
  if (!HasLabel(steps, "planning")) total = std::min(total, 70);
  if (!HasLabel(steps, "context")) total = std::min(total, 75);
  if (!HasLabel(steps, "ethics")) total = std::min(total, 82);
  if (final_answer.size() < 80) total = std::min(total, 55);
  if (self_review.size() < 60) total = std::min(total, 72);
  if (!has_code) total = std::min(total, 70);
  return Clamp(total, 0, 100);
}

}  // namespace

std::string AnalyzeSessionJson(const std::string& body) {
  const std::string session_id = ExtractString(body, "id").empty() ? ExtractString(body, "session_id") : ExtractString(body, "id");
  const auto steps = ExtractSteps(body);
  const std::string final_answer = ExtractString(body, "final_answer");
  const std::string self_review = ExtractString(body, "self_review");
  const std::string code = ExtractString(body, "code");
  const bool has_code = !code.empty() || HasAny(body, {"server.js", "main.go", "app.py", "solution.js"});

  std::vector<Dimension> dims;
  dims.push_back(MakeDim("Понимание задачи", 2 + (steps.empty() ? 0 : 2) + (HasLabel(steps, "clarification") ? 7 : 0) + (self_review.empty() ? 0 : 3), 15, HasLabel(steps, "clarification") ? "кандидат зафиксировал ограничения или критерии" : "ограничения и критерии почти не зафиксированы"));
  dims.push_back(MakeDim("Декомпозиция", 1 + (HasLabel(steps, "planning") ? 8 : 0) + (steps.size() >= 3 ? 4 : 0) + (steps.size() >= 5 ? 2 : 0), 15, HasLabel(steps, "planning") ? "есть планирование или разбиение на этапы" : "задача почти не разбита на подзадачи"));
  dims.push_back(MakeDim("Качество контекста", 2 + (HasLabel(steps, "context") ? 7 : 0) + (HasLongPrompt(steps) ? 4 : 0), 15, HasLabel(steps, "context") ? "модели задан контекст или формат" : "контекста для модели мало"));
  dims.push_back(MakeDim("Итеративность", (steps.size() >= 2 ? 3 : 0) + (HasLabel(steps, "iteration") ? 5 : 0) + (steps.size() >= 4 ? 2 : 0), 10, HasLabel(steps, "iteration") ? "есть улучшение или исправление результата" : "нет явного улучшения результата"));
  dims.push_back(MakeDim("Контроль ошибок и галлюцинаций", (HasLabel(steps, "verification") ? 9 : 0) + (HasAny(self_review, {"ошиб", "провер", "огранич", "риск", "test", "валид"}) ? 4 : 0) + (has_code ? 2 : 0), 15, HasLabel(steps, "verification") ? "есть проверка, тесты или поиск ошибок" : "нет заметной проверки результата"));
  dims.push_back(MakeDim("Финальный артефакт", (final_answer.size() > 80 ? 5 : 0) + (has_code ? 6 : 0) + (HasAny(body, {"README.md", "readme"}) ? 2 : 0) + (HasAny(body, {"test", "тест"}) ? 2 : 0), 15, has_code ? "workspace содержит рабочие файлы" : "рабочие файлы выражены слабо"));
  dims.push_back(MakeDim("Этика, privacy и bias", (HasLabel(steps, "ethics") ? 7 : 0) + (HasAny(self_review, {"privacy", "приват", "bias", "этик", "предвз"}) ? 3 : 0), 10, HasLabel(steps, "ethics") ? "есть упоминание privacy/bias/ethics" : "этические ограничения не отражены"));
  dims.push_back(MakeDim("Полезность результата", (has_code ? 2 : 0) + (final_answer.size() > 200 ? 2 : 0) + (self_review.size() > 100 ? 1 : 0), 5, has_code ? "результат можно использовать как рабочий артефакт" : "польза результата ограничена"));

  int total = 0;
  for (const auto& d : dims) total += d.score;
  total = ApplyCaps(total, steps, final_answer, self_review, has_code);

  std::vector<std::string> strengths;
  std::vector<std::string> weaknesses;
  std::vector<std::string> red_flags;
  if (HasLabel(steps, "planning")) strengths.push_back("Кандидат использовал планирование и декомпозицию.");
  if (HasLabel(steps, "verification")) strengths.push_back("Кандидат пытался проверять ошибки и ограничения.");
  if (has_code) strengths.push_back("В workspace есть рабочие кодовые файлы.");
  if (!HasLabel(steps, "verification")) weaknesses.push_back("Нет сильной проверки результата и контроля галлюцинаций.");
  if (!HasLabel(steps, "ethics")) weaknesses.push_back("Не отражены privacy/bias/ethics ограничения.");
  if (steps.size() < 3) red_flags.push_back("Слишком короткая prompt-chain для высокой оценки.");
  if (!has_code) red_flags.push_back("Нет убедительного рабочего файла.");
  if (strengths.empty()) strengths.push_back("Кандидат начал процесс взаимодействия с ИИ и оставил trace в системе.");

  std::string recommendation = "требуется ручная перепроверка или повторное задание";
  if (total >= 80 && red_flags.size() <= 1) recommendation = "автоматически допустить к следующему этапу";
  else if (total >= 65) recommendation = "допустить с дополнительной проверкой слабых зон";
  else if (total < 45) recommendation = "не хватает evidence для прохождения автоматического этапа";

  std::ostringstream os;
  os << "{";
  os << "\"report_id\":\"report-" << EscapeJson(session_id) << "\",";
  os << "\"session_id\":\"" << EscapeJson(session_id) << "\",";
  os << "\"total_score\":" << total << ",";
  os << "\"dimensions\":[";
  for (size_t i = 0; i < dims.size(); ++i) { if (i) os << ","; os << DimJson(dims[i]); }
  os << "],";
  os << "\"prompt_labels\":" << PromptLabelsJson(steps) << ",";
  os << "\"strengths\":" << StringArrayJson(strengths) << ",";
  os << "\"weaknesses\":" << StringArrayJson(weaknesses) << ",";
  os << "\"red_flags\":" << StringArrayJson(red_flags) << ",";
  os << "\"recommendation\":\"" << EscapeJson(recommendation) << "\",";
  os << "\"summary\":\"Кандидат получил " << total << "/100. Оценка построена по prompt-chain, workspace, финальному ответу и self-review.\",";
  os << "\"next_interview_questions\":" << StringArrayJson({
    "Как вы проверяли, что Алиса не допустила ошибку?",
    "Какие ограничения задачи вы явно передали модели?",
    "Что бы вы изменили в prompt-chain при повторной попытке?",
    "Какие части результата требуют ручной проверки?",
    "Где возможен bias или ложная уверенность модели?"}) << ",";
  os << "\"practical_tasks\":" << StringArrayJson({
    "Добавьте smoke-test к созданному артефакту.",
    "Попросите Алису найти 3 риска решения и подтвердите их вручную.",
    "Улучшите промпт так, чтобы он содержал формат, ограничения и критерии успеха."}) << ",";
  os << "\"ethics_privacy_notes\":" << StringArrayJson({
    "Оценивается только процесс работы в этой сессии.",
    "Система не использует личные характеристики кандидата.",
    "Автоматическая рекомендация должна быть объяснимой и проверяемой."}) << ",";
  os << "\"generated_by\":\"aca-cpp-analyzer\"";
  os << "}";
  return os.str();
}

}  // namespace aca
