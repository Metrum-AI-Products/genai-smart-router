# Competitive Notes

These notes support public competitive docs. Keep public claims primary-source-first and source-dated. Do not copy exact pricing into public docs unless it was revalidated during that task.

Last reviewed: 2026-06-19.

## Public Positioning Rule

Position Smart LLM Router as a governed enterprise LLM gateway with deployment-defined routing control, detailed usage/cost accounting, private upstream support, and agent-client compatibility.

Do not position it as:

- a public model marketplace;
- a generic observability SaaS;
- a full enterprise admin suite with SSO/session management and compliance workflow automation; the repository includes a focused authenticated browser reporting surface for usage/performance/cost operations;
- an automatic quality oracle for every model/prompt.

## Primary Source Baseline

Use these primary links for public docs:

- LiteLLM Enterprise: https://docs.litellm.ai/docs/enterprise
- Bifrost overview: https://docs.getbifrost.ai/overview
- Bifrost GitHub: https://github.com/maximhq/bifrost
- OpenRouter pricing: https://openrouter.ai/pricing
- Portkey pricing: https://portkey.ai/pricing
- Helicone pricing: https://www.helicone.ai/pricing
- Cloudflare AI Gateway pricing: https://developers.cloudflare.com/ai-gateway/reference/pricing/
- Cloudflare AI Gateway limits: https://developers.cloudflare.com/ai-gateway/reference/limits/
- Kong AI Gateway: https://konghq.com/products/kong-ai-gateway
- Kong pricing: https://konghq.com/pricing
- TrueFoundry AI Gateway: https://www.truefoundry.com/ai-gateway
- TrueFoundry pricing: https://www.truefoundry.com/pricing
- Martian: https://withmartian.com/

## Claim Safety

Safe public claims:

- Smart LLM Router supports deployment-defined model groups.
- Smart LLM Router supports OpenAI Chat, OpenAI Responses, and Anthropic Messages API shapes.
- Smart LLM Router supports dialect-specific tool routing based on configured metadata.
- Smart LLM Router supports image-aware routing when upstream metadata includes validated image modality.
- Smart LLM Router records request-time cost values in usage rows.
- Smart LLM Router can route to private OpenAI-compatible upstreams such as vLLM and SGLang when configured and validated.

Avoid or qualify:

- Exact competitor prices.
- Claims that competitors lack a feature unless primary docs clearly show that.
- Claims that Smart LLM Router has enterprise dashboard/SSO/compliance UI features unless implemented.
- Claims that the router automatically improves quality or cost for every workload.
