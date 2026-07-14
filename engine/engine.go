// Copyright 2019 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Drone Non-Commercial License
// that can be found in the LICENSE file.

package engine

//go:generate mockgen -source=engine.go -destination=mocks/engine.go

import (
	"context"
	"io"
)

const (
	// EnvironmentFileVariable 是命令步骤向依赖后继步骤发布变量的环境文件路径变量名。
	EnvironmentFileVariable = "CI_ENV"

	// DroneEnvironmentFileVariable 保留 Drone 兼容别名。
	DroneEnvironmentFileVariable = "DRONE_ENV"

	// EnvironmentFilePath 是单个步骤容器私有的环境文件路径。
	EnvironmentFilePath = "/tmp/awecloud-ci/env"

	// EnvironmentFilePathWindows 是 Windows 下的环境文件路径。
	EnvironmentFilePathWindows = "C:\\awecloud-ci\\env"
)

// Engine defines a runtime engine for pipeline execution.
type Engine interface {
	// Setup the pipeline environment.
	Setup(context.Context, *Spec) error

	// Create creates the pipeline state.
	Create(context.Context, *Spec, *Step) error

	// Start the pipeline step.
	Start(context.Context, *Spec, *Step) error

	// Wait for the pipeline step to complete and returns the completion results.
	Wait(context.Context, *Spec, *Step) (*State, error)

	// Tail the pipeline step logs.
	Tail(context.Context, *Spec, *Step) (io.ReadCloser, error)

	// ReadFile 在清理前从已完成的步骤容器导出文件。
	// 文件不存在时返回 nil 字节切片和 nil 错误。
	ReadFile(context.Context, *Spec, *Step, string) ([]byte, error)

	// Destroy the pipeline environment.
	Destroy(context.Context, *Spec) error
}
