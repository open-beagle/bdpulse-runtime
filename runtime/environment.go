package runtime

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/open-beagle/bdpulse-runtime/engine"
)

const (
	maxEnvironmentFileSize  = 64 * 1024
	maxEnvironmentVariables = 256
	maxEnvironmentKeySize   = 128
	maxEnvironmentValueSize = 8 * 1024
)

var (
	environmentKey        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	environmentExpression = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
)

func (r *Runtime) prepareStep(original *engine.Step) (*engine.Step, error) {
	dynamic, err := r.inheritedEnvironment(original)
	if err != nil {
		return nil, err
	}

	step := cloneStep(original)
	for key, value := range dynamic {
		if !step.EnvOverrides[key] {
			step.Envs[key] = value
		}
	}

	if step.Docker != nil {
		var err error
		step.Docker.Image, err = resolveEnvironmentExpressions(step.Docker.Image, step.Envs)
		if err != nil {
			return nil, fmt.Errorf("step %q image: %w", step.Metadata.Name, err)
		}
	}
	for key, value := range step.Envs {
		if !strings.HasPrefix(key, "PLUGIN_") {
			continue
		}
		resolved, err := resolveEnvironmentExpressions(value, step.Envs)
		if err != nil {
			return nil, fmt.Errorf("step %q setting %q: %w", step.Metadata.Name, key, err)
		}
		step.Envs[key] = resolved
	}
	return step, nil
}

func (r *Runtime) collectEnvironment(ctx context.Context, step *engine.Step) error {
	path, ok := step.Envs[engine.EnvironmentFileVariable]
	if !ok {
		return nil
	}
	data, err := r.engine.ReadFile(ctx, r.config, step, path)
	if err != nil {
		return fmt.Errorf("step %q cannot export %s: %w", step.Metadata.Name, engine.EnvironmentFileVariable, err)
	}
	values, err := parseEnvironmentFile(data)
	if err != nil {
		return fmt.Errorf("step %q invalid %s: %w", step.Metadata.Name, engine.EnvironmentFileVariable, err)
	}
	r.mu.Lock()
	r.outputs[step.Metadata.Name] = values
	r.mu.Unlock()
	return nil
}

func (r *Runtime) inheritedEnvironment(step *engine.Step) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	values := map[string]string{}
	merge := func(name string) error {
		for key, value := range r.outputs[name] {
			if existing, ok := values[key]; ok && existing != value {
				return fmt.Errorf("step %q receives conflicting environment variable %q", step.Metadata.Name, key)
			}
			values[key] = value
		}
		return nil
	}

	if isSerial(r.config) {
		for _, candidate := range r.config.Steps {
			if candidate.Metadata.Name == step.Metadata.Name {
				break
			}
			if err := merge(candidate.Metadata.Name); err != nil {
				return nil, err
			}
		}
		return values, nil
	}

	visited := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true
		for _, candidate := range r.config.Steps {
			if candidate.Metadata.Name != name {
				continue
			}
			for _, dependency := range candidate.DependsOn {
				if err := visit(dependency); err != nil {
					return err
				}
			}
			return merge(name)
		}
		return nil
	}
	for _, dependency := range step.DependsOn {
		if err := visit(dependency); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func cloneStep(from *engine.Step) *engine.Step {
	to := *from
	to.Envs = make(map[string]string, len(from.Envs))
	for key, value := range from.Envs {
		to.Envs[key] = value
	}
	if len(from.EnvOverrides) > 0 {
		to.EnvOverrides = make(map[string]bool, len(from.EnvOverrides))
		for key, value := range from.EnvOverrides {
			to.EnvOverrides[key] = value
		}
	}
	if from.Docker != nil {
		docker := *from.Docker
		to.Docker = &docker
	}
	return &to
}

func resolveEnvironmentExpressions(value string, environment map[string]string) (string, error) {
	var unresolved string
	resolved := environmentExpression.ReplaceAllStringFunc(value, func(expression string) string {
		parts := environmentExpression.FindStringSubmatch(expression)
		resolved, ok := environment[parts[1]]
		if !ok {
			unresolved = parts[1]
			return expression
		}
		return resolved
	})
	if unresolved != "" {
		return "", fmt.Errorf("undefined environment variable %q", unresolved)
	}
	return resolved, nil
}

func parseEnvironmentFile(data []byte) (map[string]string, error) {
	if len(data) > maxEnvironmentFileSize {
		return nil, fmt.Errorf("file exceeds %d bytes", maxEnvironmentFileSize)
	}
	values := map[string]string{}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || trimmed[0] == '#' {
			continue
		}
		parts := bytes.SplitN(line, []byte{'='}, 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected KEY=VALUE")
		}
		key, value := string(parts[0]), string(parts[1])
		if !environmentKey.MatchString(key) {
			return nil, fmt.Errorf("invalid variable name %q", key)
		}
		if isProtectedEnvironment(key) {
			return nil, fmt.Errorf("protected variable %q", key)
		}
		if len(key) > maxEnvironmentKeySize || len(value) > maxEnvironmentValueSize {
			return nil, fmt.Errorf("variable %q exceeds size limit", key)
		}
		if _, exists := values[key]; !exists && len(values) == maxEnvironmentVariables {
			return nil, fmt.Errorf("file exceeds %d variables", maxEnvironmentVariables)
		}
		values[key] = value
	}
	return values, nil
}

func isProtectedEnvironment(key string) bool {
	for _, prefix := range []string{"DRONE_", "CI_", "AWE_", "PLUGIN_"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
