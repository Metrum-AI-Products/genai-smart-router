#!/usr/bin/env python3
"""Regression coverage for the non-deployment GitHub Actions workflow check."""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from validate_eks_staging_workflow import (  # noqa: E402
    PINNED_CHECKOUT,
    DYNAMIC_EXECUTABLE,
    REVIEWED_RUNNER,
    WORKFLOW_WATCHED_PATHS,
    expected_contract_semantic_lines,
    forbidden_deployment_commands,
    script_runs_eks_staging_contract,
    validate_staging_oidc_trust_policy,
    validate_staging_workflow_contract,
    validate_canonical_workflow_structure,
    validate_workflow_path_filters,
    validate_workflow_permissions,
    validate_workflow_runner,
    workflow_action_violations,
    workflow_deployment_command_violations,
    workflow_execution_configuration_violations,
    workflow_semantic_lines,
    workflow_unsupported_yaml_syntax_lines,
    workflow_trigger_paths,
)


def workflow(run: str) -> str:
    return f"""jobs:
  contract:
    steps:
      - name: contract
        run: {run}
"""


def expect_rejected(label: str, callback, contains: str | None = None) -> None:
    try:
        callback()
    except ValueError as exc:
        if contains is not None:
            assert contains in str(exc), (label, str(exc))
        return
    raise AssertionError(f"expected {label} to be rejected")


def main() -> int:
    cases = {
        "kubectl apply -f manifest.yaml": {"kubectl"},
        "KUBECONFIG=/tmp/config kubectl apply -f manifest.yaml": {"kubectl"},
        "AWS_REGION=us-east-1 aws sts get-caller-identity": {"aws"},
        "PATH=/tmp:$PATH kustomize build deploy/kubernetes": {"kustomize"},
        "env AWS_REGION=us-east-1 kubectl apply -f manifest.yaml": {"kubectl"},
        "env -i PATH=$PATH kustomize build deploy/kubernetes": {"kustomize"},
        "echo preparing && kubectl apply -f manifest.yaml": {"kubectl"},
        "if test -n \"$READY\"; then kubectl apply -f manifest.yaml; fi": {"kubectl"},
        "sudo -u runner kustomize build deploy/kubernetes": {"kustomize"},
        "command kubectl get pods": {"kubectl"},
        "timeout -k 5 30 aws sts get-caller-identity": {"aws"},
        "sh -c 'echo preparing && kubectl apply -f manifest.yaml'": {"kubectl"},
        "eval 'kustomize build deploy/kubernetes'": {"kustomize"},
        "echo $(kubectl get namespace default)": {"kubectl"},
        "cat <(kustomize build deploy/kubernetes)": {"kustomize"},
        "cat <<EOF\n$(kubectl apply -f manifest.yaml)\nEOF": {"kubectl"},
        "cat <<EOF\n`kustomize build deploy/kubernetes`\nEOF": {"kustomize"},
        "cat <<EOF\n'$(aws sts get-caller-identity)'\nEOF": {"aws"},
    }
    for script, expected in cases.items():
        assert forbidden_deployment_commands(script) == expected, script

    allowed = (
        "echo kubectl apply is prohibited in the release workflow",
        "printf '%s\\n' kustomize",
        "if command -v kubectl; then echo installed; fi",
        "echo '${kubectl}' ${kubectl} $((kubectl + 1))",
        "case $kind in kubectl) echo a label, not a command ;; other) echo ok ;; esac",
        "cat <<'EXAMPLE'\nkubectl apply -f manifest.yaml\nEXAMPLE",
        "cat <<'EXAMPLE'\n$(kubectl apply -f manifest.yaml)\n`kustomize build deploy/kubernetes`\nEXAMPLE",
        "cat <<\\EXAMPLE\n$(kubectl apply -f manifest.yaml)\nEXAMPLE",
        "cat <<EOF\n\\$(kubectl apply -f manifest.yaml)\nEOF",
        "kubectl() { echo helper; }",
    )
    for script in allowed:
        assert not forbidden_deployment_commands(script), script

    assert forbidden_deployment_commands("TOOL=kubectl; $TOOL apply -f manifest.yaml") == {DYNAMIC_EXECUTABLE}
    assert forbidden_deployment_commands("TOOL=aws; ${TOOL} sts get-caller-identity") == {DYNAMIC_EXECUTABLE}
    assert forbidden_deployment_commands("TOOL=kustomize; command $TOOL build deploy/kubernetes") == {DYNAMIC_EXECUTABLE}

    literal_block = """jobs:
  contract:
    steps:
      - run: |
          echo preparing && kubectl apply -f manifest.yaml
"""
    folded_text = """jobs:
  contract:
    steps:
      - run: >
          echo kubectl
          is only documentation text
      # kubectl in a YAML comment is not a command
"""
    unquoted_heredoc = """jobs:
  contract:
    steps:
      - run: |
          cat <<EOF
          $(kubectl apply -f manifest.yaml)
          EOF
"""
    quoted_heredoc = unquoted_heredoc.replace("<<EOF", "<<'EOF'")
    assert workflow_deployment_command_violations(literal_block) == [(5, {"kubectl"})]
    assert not workflow_deployment_command_violations(folded_text)
    assert workflow_deployment_command_violations(unquoted_heredoc) == [(5, {"kubectl"})]
    assert not workflow_deployment_command_violations(quoted_heredoc)

    allowed_action = f"""jobs:
  contract:
    steps:
      - uses: {PINNED_CHECKOUT} # immutable checkout action
"""
    unpinned_action = allowed_action.replace(PINNED_CHECKOUT, "actions/checkout@v4")
    deployment_action = allowed_action.replace(PINNED_CHECKOUT, "acme/deploy-to-eks@" + "a" * 40)
    spaced_allowed_action = allowed_action.replace("uses:", "uses :")
    spaced_unpinned_action = unpinned_action.replace("uses:", "uses :")
    quoted_allowed_action = allowed_action.replace("uses:", '"uses":')
    quoted_unpinned_action = unpinned_action.replace("uses:", "'uses':")
    uses_text_in_run = """jobs:
  contract:
    steps:
      - run: |
          echo 'uses: acme/deploy-to-eks@v1 is an example string'
"""
    assert not workflow_action_violations(allowed_action)
    assert [action for _, action in workflow_action_violations(unpinned_action)] == ["actions/checkout@v4"]
    assert [action for _, action in workflow_action_violations(deployment_action)] == ["acme/deploy-to-eks@" + "a" * 40]
    assert not workflow_action_violations(spaced_allowed_action)
    assert [action for _, action in workflow_action_violations(spaced_unpinned_action)] == ["actions/checkout@v4"]
    assert not workflow_action_violations(quoted_allowed_action)
    assert [action for _, action in workflow_action_violations(quoted_unpinned_action)] == ["actions/checkout@v4"]
    assert not workflow_action_violations(uses_text_in_run)

    spaced_run = """jobs:
  contract:
    steps:
      - run : |
          kubectl apply -f manifest.yaml
"""
    dynamic_run = """jobs:
  contract:
    steps:
      - run: TOOL=kubectl; $TOOL apply -f manifest.yaml
"""
    quoted_run = """jobs:
  contract:
    steps:
      - "run": kubectl apply -f manifest.yaml
"""
    assert workflow_deployment_command_violations(spaced_run) == [(5, {"kubectl"})]
    assert workflow_deployment_command_violations(dynamic_run) == [(4, {DYNAMIC_EXECUTABLE})]
    assert workflow_deployment_command_violations(quoted_run) == [(4, {"kubectl"})]

    validate_workflow_permissions("""permissions :
  contents : read
jobs:
  contract:
    runs-on: ubuntu-24.04
""")
    expect_rejected("inline permission scalar", lambda: validate_workflow_permissions("permissions: read-all\njobs: {}\n"))
    expect_rejected("write permission", lambda: validate_workflow_permissions("permissions:\n  contents: write\njobs: {}\n"))
    expect_rejected("job permission override", lambda: validate_workflow_permissions("""permissions:
  contents: read
jobs:
  contract:
    permissions:
      id-token: write
"""))
    validate_workflow_permissions('''"permissions":
  "contents": read
jobs:
  contract:
    runs-on: ubuntu-24.04
''')
    expect_rejected("quoted job permission override", lambda: validate_workflow_permissions('''permissions:
  contents: read
jobs:
  contract:
    'permissions':
      id-token: write
'''))

    trust = json.loads((ROOT / "deploy/aws/github-oidc-staging-role-trust-policy.example.json").read_text(encoding="utf-8"))
    validate_staging_oidc_trust_policy(trust)
    mutations = {
        "wrong-effect": lambda statement: statement.update({"Effect": "Deny"}),
        "wrong-principal": lambda statement: statement.update({"Principal": {"AWS": "*"}}),
        "wrong-action": lambda statement: statement.update({"Action": "sts:AssumeRole"}),
        "wrong-audience": lambda statement: statement["Condition"]["StringEquals"].update({"token.actions.githubusercontent.com:aud": "other"}),
        "wrong-subject": lambda statement: statement["Condition"]["StringEquals"].update({"token.actions.githubusercontent.com:sub": "repo:sysadmin-metrum-ai/genai-smart-router:environment:*"}),
    }
    for label, mutate in mutations.items():
        candidate = json.loads(json.dumps(trust))
        mutate(candidate["Statement"][0])
        expect_rejected(label, lambda candidate=candidate: validate_staging_oidc_trust_policy(candidate))
    broad_extra_allow = json.loads(json.dumps(trust))
    broad_extra_allow["Statement"].append({"Effect": "Allow", "Principal": "*", "Action": "sts:AssumeRoleWithWebIdentity"})
    expect_rejected("broad extra allow", lambda: validate_staging_oidc_trust_policy(broad_extra_allow))
    extra_deny = json.loads(json.dumps(trust))
    extra_deny["Statement"].append({"Effect": "Deny", "Principal": "*", "Action": "sts:AssumeRoleWithWebIdentity"})
    expect_rejected("extra OIDC statement", lambda: validate_staging_oidc_trust_policy(extra_deny))

    assert script_runs_eks_staging_contract("make ci-eks-staging-contract")
    assert script_runs_eks_staging_contract("\nmake ci-eks-staging-contract\n")
    assert not script_runs_eks_staging_contract("echo make ci-eks-staging-contract")
    assert not script_runs_eks_staging_contract("make ci-eks-staging-contract || true")
    assert not script_runs_eks_staging_contract("if true; then make ci-eks-staging-contract; fi")
    assert not script_runs_eks_staging_contract("# make ci-eks-staging-contract")
    contract_invocation_bypasses = (
        "MAKEFILES=untrusted.mk make ci-eks-staging-contract",
        "MAKEFLAGS=-funtrusted.mk make ci-eks-staging-contract",
        "PATH=/tmp/untrusted-bin make ci-eks-staging-contract",
        "PYTHONPATH=/tmp/untrusted-python make ci-eks-staging-contract",
        "./make ci-eks-staging-contract",
        "subdir/make ci-eks-staging-contract",
        "/tmp/untrusted-bin/make ci-eks-staging-contract",
    )
    for invocation in contract_invocation_bypasses:
        assert not script_runs_eks_staging_contract(invocation), invocation
    checked_workflow = (ROOT / ".github/workflows/eks-staging-contract.yml").read_text(encoding="utf-8")
    canonical_contract_workflow = "\n".join(expected_contract_semantic_lines()) + "\n"
    quoted_contract_workflow = (
        canonical_contract_workflow.replace("permissions:", '"permissions":')
        .replace("  contents:", '  "contents":')
        .replace("        uses:", '        "uses":')
        .replace("        run:", '        "run":')
    )
    contract_run_line = '        "run": make ci-eks-staging-contract'
    checkout_uses_line = f'        "uses": {PINNED_CHECKOUT}'
    assert workflow_semantic_lines(quoted_contract_workflow) == expected_contract_semantic_lines()
    validate_canonical_workflow_structure(quoted_contract_workflow)
    validate_staging_workflow_contract(
        checked_workflow,
        (ROOT / "Makefile").read_text(encoding="utf-8"),
        trust,
    )
    validate_staging_workflow_contract(
        quoted_contract_workflow,
        (ROOT / "Makefile").read_text(encoding="utf-8"),
        trust,
    )
    for invocation in contract_invocation_bypasses:
        expect_rejected(
            f"contract invocation override: {invocation}",
            lambda invocation=invocation: validate_staging_workflow_contract(
                quoted_contract_workflow.replace("make ci-eks-staging-contract", invocation),
                (ROOT / "Makefile").read_text(encoding="utf-8"),
                trust,
            ),
            "exactly one run step",
        )
    assert not workflow_execution_configuration_violations(quoted_contract_workflow)
    validate_workflow_runner(quoted_contract_workflow)
    execution_configuration_cases = {
        "workflow startup environment": quoted_contract_workflow.replace(
            "jobs:\n",
            "env:\n  BASH_ENV: /tmp/untrusted-startup\njobs:\n",
        ),
        "quoted workflow startup environment": quoted_contract_workflow.replace(
            "jobs:\n",
            '"env":\n  BASH_ENV: /tmp/untrusted-startup\njobs:\n',
        ),
        "job startup environment": quoted_contract_workflow.replace(
            "  contract:\n",
            "  contract:\n    env:\n      PATH: /tmp/untrusted-bin\n",
        ),
        "step startup environment": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line
            + "\n"
            '        env:\n'
            '          BASH_ENV: /tmp/untrusted-startup',
        ),
        "workflow default shell": quoted_contract_workflow.replace(
            "jobs:\n",
            "defaults:\n  run:\n    shell: bash\njobs:\n",
        ),
        "job default shell": quoted_contract_workflow.replace(
            "  contract:\n",
            "  contract:\n    defaults:\n      run:\n        shell: bash\n",
        ),
        "step shell": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        shell: bash",
        ),
        "step working directory": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        working-directory: scripts",
        ),
        "job container": quoted_contract_workflow.replace(
            "  contract:\n",
            "  contract:\n    container: ghcr.io/example/untrusted:latest\n",
        ),
        "job services": quoted_contract_workflow.replace(
            "  contract:\n",
            "  contract:\n    services:\n      helper:\n        image: ghcr.io/example/untrusted:latest\n",
        ),
        "checkout source override": quoted_contract_workflow.replace(
            checkout_uses_line,
            checkout_uses_line
            + "\n"
            '        with:\n'
            '          repository: attacker/controlled',
        ),
        "job environment context": quoted_contract_workflow.replace(
            "  contract:\n",
            "  contract:\n    environment: staging\n",
        ),
        "background and cancel lifecycle": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line
            + "\n"
            '        id: contract\n'
            '        background: true\n'
            '        cancel: contract',
        ),
        "wait lifecycle": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        wait: contract",
        ),
        "wait all lifecycle": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        wait-all: true",
        ),
        "parallel lifecycle": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        parallel: true",
        ),
        "step condition": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        if: true",
        ),
        "step continue on error": quoted_contract_workflow.replace(
            contract_run_line,
            contract_run_line + "\n        continue-on-error: true",
        ),
    }
    for label, candidate in execution_configuration_cases.items():
        expect_rejected(
            label,
            lambda candidate=candidate: validate_staging_workflow_contract(
                candidate,
                (ROOT / "Makefile").read_text(encoding="utf-8"),
                trust,
            ),
            "execution-affecting",
        )
    expect_rejected(
        "self-hosted runner",
        lambda: validate_staging_workflow_contract(
            quoted_contract_workflow.replace(REVIEWED_RUNNER, "self-hosted"),
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "reviewed GitHub-hosted runner",
    )
    expect_rejected(
        "alternate hosted runner",
        lambda: validate_staging_workflow_contract(
            quoted_contract_workflow.replace(REVIEWED_RUNNER, "ubuntu-latest"),
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "reviewed GitHub-hosted runner",
    )
    expect_rejected(
        "missing runner",
        lambda: validate_staging_workflow_contract(
            quoted_contract_workflow.replace(f"    runs-on: {REVIEWED_RUNNER}\n", ""),
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "reviewed GitHub-hosted runner",
    )
    validate_workflow_path_filters(checked_workflow)
    trigger_paths = workflow_trigger_paths(checked_workflow)
    assert trigger_paths == {
        "pull_request": WORKFLOW_WATCHED_PATHS,
        "push": WORKFLOW_WATCHED_PATHS,
    }
    missing_base_path = checked_workflow.replace("      - deploy/kubernetes/base/**\n", "", 1)
    expect_rejected(
        "missing Kubernetes base trigger path",
        lambda: validate_workflow_path_filters(missing_base_path),
        "missing deploy/kubernetes/base/**",
    )
    extra_path = checked_workflow.replace(
        "      - deploy/kubernetes/overlays/metrum-staging/**\n",
        "      - deploy/kubernetes/overlays/metrum-staging/**\n      - docs/**\n",
        1,
    )
    expect_rejected(
        "unreviewed trigger path",
        lambda: validate_workflow_path_filters(extra_path),
        "unexpected docs/**",
    )
    inline_run_continuation = quoted_contract_workflow.replace(
        contract_run_line,
        contract_run_line
        + "\n"
        '          ; kubectl apply -f deploy.yaml',
    )
    expect_rejected(
        "inline run scalar continuation",
        lambda: validate_staging_workflow_contract(
            inline_run_continuation,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "continuation lines",
    )
    inline_run_sibling = quoted_contract_workflow.replace(
        contract_run_line,
        contract_run_line
        + "\n"
        '        name: Verify the staging delivery contract',
    )
    expect_rejected(
        "unreviewed step sibling",
        lambda: validate_staging_workflow_contract(
            inline_run_sibling,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "canonical contract structure",
    )
    extra_make_run = quoted_contract_workflow.replace(
        contract_run_line,
        contract_run_line
        + "\n"
        '      - name: Unapproved rollback\n'
        '        "run": make eks-rollback-staging EKS_CONFIRM=STAGING_APPLY',
    )
    expect_rejected(
        "additional mutating Make run step",
        lambda: validate_staging_workflow_contract(
            extra_make_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "exactly one run step",
    )
    extra_interpreter_run = quoted_contract_workflow.replace(
        contract_run_line,
        contract_run_line
        + "\n"
        '      - name: Unapproved interpreter\n'
        '        "run": python3 -c \'raise SystemExit(0)\'',
    )
    expect_rejected(
        "additional interpreter run step",
        lambda: validate_staging_workflow_contract(
            extra_interpreter_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "exactly one run step",
    )
    flow_style_action = quoted_contract_workflow + "      - { uses: acme/deploy-to-eks@" + "a" * 40 + " }\n"
    expect_rejected(
        "flow-style action step",
        lambda: validate_staging_workflow_contract(
            flow_style_action,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "flow-style mappings",
    )
    flow_style_run = quoted_contract_workflow + "      - { run: kubectl apply -f deploy.yaml }\n"
    expect_rejected(
        "flow-style run step",
        lambda: validate_staging_workflow_contract(
            flow_style_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "flow-style mappings",
    )
    explicit_mapping_run = quoted_contract_workflow + "      - ? run\n        : kubectl apply -f deploy.yaml\n"
    assert workflow_unsupported_yaml_syntax_lines(explicit_mapping_run) == [
        len(quoted_contract_workflow.splitlines()) + 1,
        len(quoted_contract_workflow.splitlines()) + 2,
    ]
    expect_rejected(
        "explicit YAML mapping run step",
        lambda: validate_staging_workflow_contract(
            explicit_mapping_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "unsupported YAML node syntax",
    )
    tagged_run = quoted_contract_workflow + "      - !!str run: kubectl apply -f deploy.yaml\n"
    expect_rejected(
        "tagged YAML run step",
        lambda: validate_staging_workflow_contract(
            tagged_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "unsupported YAML node syntax",
    )
    anchor_alias_condition = quoted_contract_workflow.replace(
        "      - name: Verify EKS staging delivery contract",
        "      - name: &if_key if",
    ).replace(
        contract_run_line,
        contract_run_line + "\n      - *if_key: false",
    )
    expect_rejected(
        "anchor alias condition",
        lambda: validate_staging_workflow_contract(
            anchor_alias_condition,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "unsupported YAML node syntax",
    )
    merge_alias = quoted_contract_workflow.replace(
        "jobs:\n",
        "defaults: &defaults\n  if: false\njobs:\n",
    ).replace(
        "  contract:\n",
        "  contract:\n    <<: *defaults\n",
    )
    expect_rejected(
        "merge alias",
        lambda: validate_staging_workflow_contract(
            merge_alias,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "unsupported YAML node syntax",
    )
    document_marker = quoted_contract_workflow + "---\n"
    expect_rejected(
        "YAML document marker",
        lambda: validate_staging_workflow_contract(
            document_marker,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "unsupported YAML node syntax",
    )
    matrix_data_run = quoted_contract_workflow.replace(
        "      - name: Verify EKS staging delivery contract\n" + contract_run_line,
        "      - name: Matrix data only",
    ).replace(
        "    timeout-minutes: 10\n",
        "    timeout-minutes: 10\n"
        "    strategy:\n"
        "      matrix:\n"
        "        include:\n"
        "          - run: make ci-eks-staging-contract\n",
    )
    expect_rejected(
        "matrix data run",
        lambda: validate_staging_workflow_contract(
            matrix_data_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "canonical contract structure",
    )
    block_run = quoted_contract_workflow.replace(
        contract_run_line,
        '        "run": |\n          make ci-eks-staging-contract',
    )
    expect_rejected(
        "block scalar run",
        lambda: validate_staging_workflow_contract(
            block_run,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
        "block scalars",
    )
    inert_contract_reference = quoted_contract_workflow.replace(
        contract_run_line,
        '        "run": echo skipped',
    )
    expect_rejected(
        "inert contract target reference",
        lambda: validate_staging_workflow_contract(
            inert_contract_reference,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
    )
    echoed_contract_reference = inert_contract_reference.replace("echo skipped", "echo make ci-eks-staging-contract")
    expect_rejected(
        "echoed contract target reference",
        lambda: validate_staging_workflow_contract(
            echoed_contract_reference,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
    )

    invalid_workflow = quoted_contract_workflow.replace('  "contents": read', '  "contents": write')
    expect_rejected(
        "write permissions in contract",
        lambda: validate_staging_workflow_contract(
            invalid_workflow,
            (ROOT / "Makefile").read_text(encoding="utf-8"),
            trust,
        ),
    )
    with tempfile.TemporaryDirectory() as temporary:
        invalid_path = Path(temporary) / "workflow.yml"
        invalid_path.write_text(invalid_workflow, encoding="utf-8")
        command = "import pathlib, validate_eks_staging_workflow as validator; validator.WORKFLOW = pathlib.Path(__import__('sys').argv[1]); raise SystemExit(validator.main())"
        environment = {**os.environ, "PYTHONOPTIMIZE": "1", "PYTHONPATH": str(ROOT / "scripts")}
        optimized = subprocess.run(["python3", "-c", command, str(invalid_path)], cwd=ROOT, env=environment, text=True, capture_output=True)
        assert optimized.returncode == 2
        assert "permissions" in optimized.stderr
    print("EKS staging workflow command-validation regression tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
