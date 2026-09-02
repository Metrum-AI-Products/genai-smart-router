// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

package smartrouterctl_test

import (
	"strings"
	"testing"

	"smart-llmrouter/internal/smartrouterctl"
)

func TestNvidiaLLMDCompatRequiresLLMDBlock(t *testing.T) {
	intent := validNvidiaLLMDIntent()
	intent.LLMD = nil
	err := intent.Validate()
	if err == nil || !strings.Contains(err.Error(), "llmd block") {
		t.Fatalf("Validate() error = %v, want llmd block error", err)
	}
}

func validNvidiaLLMDIntent() *smartrouterctl.StackIntent {
	return &smartrouterctl.StackIntent{
		Profile:         "nvidia-llmd-compat",
		HardwareProfile: "l40s",
		GPUOperator: smartrouterctl.GPUOperatorIntent{
			Enabled: true,
		},
		LLMD: &smartrouterctl.LLMDCompatIntent{
			ModelServer: smartrouterctl.ModelServerIntent{
				Name:            "vllm-llmd-backend",
				HuggingFaceID:   "Qwen/Qwen3-1.7B",
				Image:           "vllm/vllm-openai:v0.11.0",
				GPUCount:        1,
				Port:            8000,
				MaxModelLen:     8192,
				MatchLabelKey:   "app",
				MatchLabelValue: "vllm-llmd-backend",
			},
		},
		ServingModels: []smartrouterctl.ServingModelIntent{{
			Name:          "llm-d-frontend",
			Backend:       "llm-d",
			ServedModelID: "local-llmd-chat",
			ModelGroup:    "local-llmd-chat",
		}},
	}
}

func TestNvidiaLocalServingRejectsExternalService(t *testing.T) {
	intent := validNvidiaLocalIntent("b200")
	intent.ServingModels[0].ServiceDNS = "https://vllm.example.com/v1"

	err := intent.Validate()
	if err == nil || !strings.Contains(err.Error(), "svc.cluster.local") {
		t.Fatalf("Validate() error = %v, want local Service DNS error", err)
	}
}

func TestNvidiaLocalServingValidatesHardwareModelMatrix(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*smartrouterctl.StackIntent)
		wantError string
	}{
		{
			name: "B200 requires mid-size primary",
			mutate: func(intent *smartrouterctl.StackIntent) {
				intent.ServingModels[0].ModelSizeBillions = 9
			},
			wantError: "20-40B primary",
		},
		{
			name: "B200 requires two small models",
			mutate: func(intent *smartrouterctl.StackIntent) {
				intent.ServingModels[1].ModelSizeBillions = 12
			},
			wantError: "two models at or below 9B",
		},
		{
			name: "L40S rejects large primary",
			mutate: func(intent *smartrouterctl.StackIntent) {
				intent.HardwareProfile = "l40s"
			},
			wantError: "models at or below 9B",
		},
		{
			name: "requires three models",
			mutate: func(intent *smartrouterctl.StackIntent) {
				intent.ServingModels = intent.ServingModels[:2]
			},
			wantError: "at least three serving_models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := validNvidiaLocalIntent("b200")
			test.mutate(intent)

			err := intent.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func validNvidiaLocalIntent(hardwareProfile string) *smartrouterctl.StackIntent {
	return &smartrouterctl.StackIntent{
		Profile:         "nvidia-local-serving",
		HardwareProfile: hardwareProfile,
		GPUOperator: smartrouterctl.GPUOperatorIntent{
			Enabled: true,
		},
		ServingModels: []smartrouterctl.ServingModelIntent{
			{
				Name:              "vllm-primary",
				ServedModelID:     "local-qwen38",
				HuggingFaceID:     "Qwen/Qwen3.8-27B",
				Image:             "vllm/vllm-openai:v0.11.0",
				GPUCount:          1,
				ModelGroup:        "local-qwen38",
				ModelSizeBillions: 27,
			},
			{
				Name:              "vllm-chat",
				ServedModelID:     "local-small-chat",
				HuggingFaceID:     "Qwen/Qwen3.5-4B",
				Image:             "vllm/vllm-openai:v0.11.0",
				GPUCount:          1,
				ModelGroup:        "local-small-chat",
				ModelSizeBillions: 4,
			},
			{
				Name:              "vllm-coder",
				ServedModelID:     "local-small-coder",
				HuggingFaceID:     "Qwen/Qwen3-8B",
				Image:             "vllm/vllm-openai:v0.11.0",
				GPUCount:          1,
				ModelGroup:        "local-small-coder",
				ModelSizeBillions: 8,
			},
		},
	}
}
