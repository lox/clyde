package judge

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// InterruptQuestion is the choice question whose answer becomes the model's
// interrupt. Its options are interrupt reasons and must include NoInterrupt.
const (
	InterruptQuestion = "interrupt"
	NoInterrupt       = "none"
)

// LoadQuestions reads the judge's question set (prompts/judge.yaml).
func LoadQuestions(path string) (map[string]Question, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Questions map[string]Question `yaml:"questions"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for id, q := range file.Questions {
		if err := validateQuestion(q); err != nil {
			return nil, fmt.Errorf("%s: question %q: %w", path, id, err)
		}
	}
	q, ok := file.Questions[InterruptQuestion]
	if !ok || q.Type != "choice" {
		return nil, fmt.Errorf("%s: needs a choice question named %q", path, InterruptQuestion)
	}
	if _, ok := q.Criteria.(map[string]any)[NoInterrupt]; !ok {
		return nil, fmt.Errorf("%s: question %q needs a %q option", path, InterruptQuestion, NoInterrupt)
	}
	return file.Questions, nil
}

func validateQuestion(q Question) error {
	if q.Instructions == nil {
		return fmt.Errorf("missing instructions")
	}
	switch q.Type {
	case "noul":
		if q.Criteria == nil {
			return nil
		}
		m, ok := q.Criteria.(map[string]any)
		if !ok {
			return fmt.Errorf("noul criteria must be a map with \"true\"/\"false\" keys (quote them in YAML)")
		}
		for k := range m {
			if k != "true" && k != "false" {
				return fmt.Errorf("noul criteria key %q is not \"true\" or \"false\"", k)
			}
		}
	case "choice":
		m, ok := q.Criteria.(map[string]any)
		if !ok || len(m) < 2 {
			return fmt.Errorf("choice criteria must map at least two options to descriptions")
		}
	case "score":
		l, ok := q.Criteria.([]any)
		if !ok || len(l) < 2 || len(l) > 10 {
			return fmt.Errorf("score criteria must list 2 to 10 levels")
		}
	default:
		return fmt.Errorf("unknown type %q", q.Type)
	}
	return nil
}
