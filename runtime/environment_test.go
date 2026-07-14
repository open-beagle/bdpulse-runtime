package runtime

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/open-beagle/bdpulse-runtime/engine"
)

func TestParseEnvironmentFile(t *testing.T) {
	values, err := parseEnvironmentFile([]byte("# comment\nBUILD_VERSION=v7.3.0\nBUILD_VERSION=v7.3.1\nCHANNEL=v7.3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := values["BUILD_VERSION"], "v7.3.1"; got != want {
		t.Fatalf("BUILD_VERSION = %q, want %q", got, want)
	}
	if got, want := values["CHANNEL"], "v7.3"; got != want {
		t.Fatalf("CHANNEL = %q, want %q", got, want)
	}
}

func TestParseEnvironmentFileRejectsProtectedVariable(t *testing.T) {
	_, err := parseEnvironmentFile([]byte("CI_TOKEN=secret\n"))
	if err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("error = %v, want protected variable error", err)
	}
}

func TestInheritedEnvironmentGraph(t *testing.T) {
	producer := &engine.Step{Metadata: engine.Metadata{Name: "version"}}
	consumer := &engine.Step{
		Metadata:  engine.Metadata{Name: "publish"},
		DependsOn: []string{"version"},
	}
	r := &Runtime{
		config:  &engine.Spec{Steps: []*engine.Step{producer, consumer}},
		outputs: map[string]map[string]string{"version": {"BUILD_VERSION": "v7.3.0"}},
	}

	values, err := r.inheritedEnvironment(consumer)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := values["BUILD_VERSION"], "v7.3.0"; got != want {
		t.Fatalf("BUILD_VERSION = %q, want %q", got, want)
	}
}

func TestInheritedEnvironmentRejectsConflict(t *testing.T) {
	left := &engine.Step{Metadata: engine.Metadata{Name: "left"}}
	right := &engine.Step{Metadata: engine.Metadata{Name: "right"}}
	consumer := &engine.Step{
		Metadata:  engine.Metadata{Name: "publish"},
		DependsOn: []string{"left", "right"},
	}
	r := &Runtime{
		config: &engine.Spec{Steps: []*engine.Step{left, right, consumer}},
		outputs: map[string]map[string]string{
			"left":  {"BUILD_VERSION": "v7.3.0"},
			"right": {"BUILD_VERSION": "v7.3.1"},
		},
	}

	_, err := r.inheritedEnvironment(consumer)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("error = %v, want conflict error", err)
	}
}

func TestResolveEnvironmentExpressions(t *testing.T) {
	value, err := resolveEnvironmentExpressions("repo:${{ env.BUILD_VERSION }}", map[string]string{
		"BUILD_VERSION": "v7.3.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value, "repo:v7.3.0"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
	_, err = resolveEnvironmentExpressions("repo:${{ env.MISSING }}", nil)
	if err == nil || !strings.Contains(err.Error(), "MISSING") {
		t.Fatalf("error = %v, want missing variable error", err)
	}
}

func TestRuntimePublishesEnvironmentToSerialConsumer(t *testing.T) {
	producer := &engine.Step{
		Metadata:  engine.Metadata{Name: "version"},
		Envs:      map[string]string{engine.EnvironmentFileVariable: engine.EnvironmentFilePath},
		RunPolicy: engine.RunAlways,
	}
	consumer := &engine.Step{
		Metadata:  engine.Metadata{Name: "publish"},
		Envs:      map[string]string{},
		RunPolicy: engine.RunAlways,
	}
	backend := &environmentTestEngine{
		files: map[string][]byte{"version": []byte("BUILD_VERSION=v7.3.0\n")},
	}
	runtime := New(WithEngine(backend), WithConfig(&engine.Spec{Steps: []*engine.Step{producer, consumer}}))
	if err := runtime.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := backend.created["publish"].Envs["BUILD_VERSION"], "v7.3.0"; got != want {
		t.Fatalf("BUILD_VERSION = %q, want %q", got, want)
	}
}

type environmentTestEngine struct {
	files   map[string][]byte
	created map[string]*engine.Step
}

func (e *environmentTestEngine) Setup(context.Context, *engine.Spec) error {
	e.created = map[string]*engine.Step{}
	return nil
}

func (e *environmentTestEngine) Create(_ context.Context, _ *engine.Spec, step *engine.Step) error {
	e.created[step.Metadata.Name] = cloneStep(step)
	return nil
}

func (e *environmentTestEngine) Start(context.Context, *engine.Spec, *engine.Step) error {
	return nil
}

func (e *environmentTestEngine) Wait(context.Context, *engine.Spec, *engine.Step) (*engine.State, error) {
	return &engine.State{ExitCode: 0, Exited: true}, nil
}

func (e *environmentTestEngine) Tail(context.Context, *engine.Spec, *engine.Step) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (e *environmentTestEngine) ReadFile(_ context.Context, _ *engine.Spec, step *engine.Step, _ string) ([]byte, error) {
	return e.files[step.Metadata.Name], nil
}

func (e *environmentTestEngine) Destroy(context.Context, *engine.Spec) error {
	return nil
}
