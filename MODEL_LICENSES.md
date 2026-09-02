# Model Licenses

## No bundled model weights

This repository does not contain model weights. A search of tracked files found
no common weight artifacts such as `.safetensors`, `.gguf`, `.onnx`, `.pt`,
`.pth`, `.ckpt`, `.tflite`, `.pb`, `.h5`, or model-weight `.bin` files. The
project license therefore does not grant rights to any model weights.

## Provider and model references

`config.example.yaml` contains provider model catalogs and routing examples.
Those entries are identifiers and metadata for separately operated upstream
services; they do not bundle or sublicense the referenced models. Operators
must review and comply with each provider's and model owner's terms before use.
The repository does not contain a locally authoritative, complete set of those
terms.

Capability and evaluation material in `docs/`, `scripts/`, `examples/`,
`evaluators/`, and `tests/` also accepts or shows provider/model identifiers.
These are request, smoke-test, or benchmark references rather than downloads
or grants of model rights.

## Known download surfaces

The checked-in local-serving Kubernetes examples pass these repository IDs to
vLLM with `--model`, which can cause the serving environment to obtain weights
that are not included here:

| Checked-in deployment surface | Referenced model repository ID |
| --- | --- |
| `deploy/kubernetes/overlays/k3s-amd-instinct-local-serving/serving/vllm-chat-deployment.yaml` | `Qwen/Qwen3.5-4B` |
| `deploy/kubernetes/overlays/k3s-amd-instinct-local-serving/serving/vllm-coder-deployment.yaml` | `Qwen/Qwen3-8B` |
| `deploy/kubernetes/overlays/k3s-amd-instinct-local-serving/serving/vllm-tiny-deployment.yaml` | `Qwen/Qwen3-1.7B` |
| `deploy/kubernetes/overlays/nvidia-local-serving/serving/vllm-chat-deployment.yaml` | `Qwen/Qwen3-4B-Instruct-2507` |
| `deploy/kubernetes/overlays/nvidia-local-serving/serving/vllm-coder-deployment.yaml` | `Qwen/Qwen3-8B` |
| `deploy/kubernetes/overlays/nvidia-local-serving/serving/vllm-tiny-deployment.yaml` | `Qwen/Qwen3-1.7B` |

`docs/SELF_HOSTED_UPSTREAMS.md` additionally uses model IDs such as
`qwen3-coder-tools` and `qwen-vl` as operator-supplied examples; it does not
pin a model artifact or its terms.

## Unknown terms and operator responsibility

No model card, model license, acceptable-use policy, gated-access condition,
or provider service terms were fetched or verified for this inventory. The
terms, restrictions, attribution requirements, and commercial-use permissions
for every referenced model remain unresolved here and may differ by model
version, repository revision, provider, geography, and use case.

Before downloading, deploying, fine-tuning, redistributing, or invoking a
model, the operator must identify the exact artifact and revision, retrieve its
current authoritative terms from the model owner or provider, confirm that the
intended use and distribution are permitted, and retain the required license
and notice evidence. Model access through a provider does not imply permission
to redistribute that model's weights.
