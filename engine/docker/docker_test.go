// Copyright 2019 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Drone Non-Commercial License
// that can be found in the LICENSE file.

package docker

import (
	"context"
	"strings"
	"testing"

	"github.com/open-beagle/bdpulse-runtime/engine"
)

func TestCreateRejectMissingSecret(t *testing.T) {
	backend := New(nil)
	err := backend.Create(context.Background(), &engine.Spec{}, &engine.Step{
		Docker: &engine.DockerStep{Image: "alpine"},
		Secrets: []*engine.SecretVar{{
			Name: "REGISTRY_PASSWORD",
			Env:  "PLUGIN_PASSWORD",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "REGISTRY_PASSWORD") {
		t.Fatalf("error = %v, want missing secret error", err)
	}
}
