---
title: Deployment Administration
doc_type: howto
---

# Deployment Administration

This guide is for operators who own router configuration, provider
credentials, model groups, caller access, quotas, and routine changes in a
self-managed deployment.

| Shape | Configuration owner | Activation |
|---|---|---|
| Binary or Compose | Protected host files | Validate, then restart or recreate the service |
| Kubernetes | Deployment-owned Secret | Update the Secret, then roll the Deployment |

Every deployment needs `config.yaml`, protected provider credentials in
`env.json` or an equivalent secret manager, an operator-generated runtime
license, durable state, and at least one caller identity. Never put production
runtime files in a ConfigMap or public issue.

## Validate Configuration

```bash
metrum-genai-smartrouterctl config validate --config /path/to/config.yaml
metrum-genai-smartrouterctl callers list --config /path/to/config.yaml
metrum-genai-smartrouterctl providers list --config /path/to/config.yaml
metrum-genai-smartrouterctl models list --config /path/to/config.yaml
```

The local CLI may update file-owned config, but it does not call Kubernetes or
activate remote infrastructure. Apply changes through the deployment's normal
service or cluster controls.

## Issue And Rotate Caller Tokens

Generate a token for existing active user, project, and membership records:

```bash
router-token-gen generate \
  --owner-user <user-id> \
  --project <project-id> \
  --env <environment> \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Store the cleartext token only in the caller's protected credential store and
put the generated hash row in `config.yaml`. To rotate, activate the new token,
verify `/v1/models` and one request, then mark the old row rotated or disabled.

## Local-Serving Profiles

`nvidia-local-serving` renders direct in-cluster serving targets. The separate
`nvidia-llmd-compat` profile renders an llm-d frontend target so llm-d selects
replicas behind the router-selected group. AMD Instinct uses the maintained
manual `k3s-amd-instinct-local-serving` overlay. Run each profile's offline gate
and direct plus router-level smokes before exposure.

## Change Checklist

1. Back up configuration, state, runtime-license inputs, and the usage database.
2. Validate structured config and the exact request shapes affected.
3. Activate the reviewed revision with an immutable package/image.
4. Check `/readyz`, `/version`, authenticated `/v1/models`, and representative
   caller requests.
5. Check metrics with an authorized metrics-admin caller and confirm an
   ordinary caller receives `403 metrics-forbidden`.
6. Roll back the package, config, license pair, and database together when the
   release contract requires it.

