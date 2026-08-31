package smartrouterctl_test

import (
	"strings"
	"testing"

	"smart-llmrouter/internal/smartrouterctl"
)

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
				ServedModelID:     "Qwen/Qwen3.8-27B-Instruct",
				HuggingFaceID:     "Qwen/Qwen3.8-27B-Instruct",
				Image:             "vllm/vllm-openai:v0.11.0",
				GPUCount:          1,
				ModelGroup:        "local-qwen38",
				ModelSizeBillions: 27,
			},
			{
				Name:              "vllm-chat",
				ServedModelID:     "Qwen/Qwen3.5-4B-Instruct",
				HuggingFaceID:     "Qwen/Qwen3.5-4B-Instruct",
				Image:             "vllm/vllm-openai:v0.11.0",
				GPUCount:          1,
				ModelGroup:        "local-small-chat",
				ModelSizeBillions: 4,
			},
			{
				Name:              "vllm-coder",
				ServedModelID:     "Qwen/Qwen3-Coder-8B-Instruct",
				HuggingFaceID:     "Qwen/Qwen3-Coder-8B-Instruct",
				Image:             "vllm/vllm-openai:v0.11.0",
				GPUCount:          1,
				ModelGroup:        "local-small-coder",
				ModelSizeBillions: 8,
			},
		},
	}
}
