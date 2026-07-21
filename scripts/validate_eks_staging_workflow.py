#!/usr/bin/env python3
"""Keep the checked-in EKS workflow a pinned, non-deploy contract check."""
from __future__ import annotations

import json
import re
import shlex
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github/workflows/eks-staging-contract.yml"
TRUST = ROOT / "deploy/aws/github-oidc-staging-role-trust-policy.example.json"
MAKEFILE = ROOT / "Makefile"
PINNED_CHECKOUT = "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683"
ALLOWED_ACTIONS = frozenset({PINNED_CHECKOUT})
WORKFLOW_NAME = "EKS staging contract"
CONTRACT_JOB_NAME = "contract"
CONTRACT_TIMEOUT_MINUTES = "10"
CHECKOUT_STEP_NAME = "Check out source"
CONTRACT_STEP_NAME = "Verify EKS staging delivery contract"
SUBJECT = "repo:sysadmin-metrum-ai/genai-smart-router:environment:staging"
OIDC_FEDERATED_PRINCIPAL = "arn:aws:iam::<account-id>:oidc-provider/token.actions.githubusercontent.com"
OIDC_AUDIENCE_CLAIM = "token.actions.githubusercontent.com:aud"
OIDC_SUBJECT_CLAIM = "token.actions.githubusercontent.com:sub"
OIDC_AUDIENCE = "sts.amazonaws.com"
FORBIDDEN_DEPLOYMENT_COMMANDS = frozenset({"aws", "kubectl", "kustomize"})
DYNAMIC_EXECUTABLE = "<dynamic-executable>"
REVIEWED_RUNNER = "ubuntu-24.04"
FORBIDDEN_EXECUTION_CONFIGURATION_KEYS = frozenset(
    {
        "background",
        "cancel",
        "container",
        "continue-on-error",
        "defaults",
        "environment",
        "env",
        "if",
        "parallel",
        "services",
        "shell",
        "wait",
        "wait-all",
        "with",
        "working-directory",
    }
)
WORKFLOW_WATCHED_PATHS = (
    ".github/workflows/eks-staging-contract.yml",
    "Makefile",
    "scripts/makefile_security_test.py",
    "scripts/eks_delivery.py",
    "scripts/eks_delivery_contract_test.py",
    "scripts/validate_staging_supply_chain.py",
    "scripts/validate_staging_supply_chain_test.py",
    "scripts/validate_eks_staging_workflow.py",
    "scripts/validate_eks_staging_workflow_test.py",
    "deploy/aws/github-oidc-staging-role-trust-policy.example.json",
    "deploy/aws/genai-smart-router-eks-staging-target.json",
    "deploy/kubernetes/base/**",
    "deploy/kubernetes/overlays/metrum-staging/**",
)

YAML_MAPPING_KEY = re.compile(
    r'''^(?P<indent> *)(?:- *)?(?:(?P<quoted_key>'(?:''|[^'])*'|"(?:\\.|[^"\\])*")|(?P<bare_key>[A-Za-z_][A-Za-z0-9_-]*))\s*:\s*(?P<value>.*)$'''
)
BLOCK_SCALAR = re.compile(r"^[|>][1-9+-]*\s*(?:#.*)?$")
YAML_EXPLICIT_MAPPING_KEY = re.compile(r"^\s*(?:-\s*)?\?(?:\s|$)")
YAML_EXPLICIT_MAPPING_VALUE = re.compile(r"^\s*:(?:\s|$)")
YAML_DOCUMENT_MARKER = re.compile(r"^\s*(?:---|\.\.\.)(?:\s|$)")
YAML_DIRECTIVE = re.compile(r"^\s*%")
YAML_MERGE_KEY = re.compile(r"^\s*(?:-\s*)?<<\s*:")
YAML_NODE_PREFIX = re.compile(r"^\s*(?:-\s*)?[!&*]")
YAML_FLOW_SEQUENCE = re.compile(r"^\s*(?:-\s*)?\[")
YAML_LIST_ITEM = re.compile(r"^(?P<indent> *)-\s+(?P<value>.*)$")
ASSIGNMENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")
PUNCTUATION = frozenset("|&;()<>{}")
REDIRECTIONS = frozenset({"<", ">", "<<", ">>", "<<<", "<>", "<&", ">&", ">|", "&>", "&>>"})
SEPARATORS = frozenset({";", "&&", "||", "|", "|&", "&", "(", "{"})
CLOSERS = frozenset({")", "}"})
SHELLS = frozenset({"sh", "bash", "dash", "zsh", "fish"})


def _split_punctuation(token: str) -> list[str]:
    """Split a shlex punctuation run while retaining shell operator boundaries."""
    if not token or any(character not in PUNCTUATION for character in token):
        return [token]
    operators = (";;&", "&>>", "&&", "||", "|&", ">>", "<<", "<<<", "<>", "<&", ">&", "&>", ";;", ";&", ">|", ";", "|", "&", "(", ")", "<", ">", "{", "}")
    parts: list[str] = []
    while token:
        operator = next((value for value in operators if token.startswith(value)), None)
        if operator is None:
            parts.append(token[0])
            token = token[1:]
        else:
            parts.append(operator)
            token = token[len(operator):]
    return parts


def _lex_shell(script: str) -> list[str]:
    lexer = shlex.shlex(script, posix=True, punctuation_chars="|&;()<>{}")
    lexer.whitespace_split = True
    lexer.commenters = "#"
    tokens: list[str] = []
    for token in lexer:
        tokens.extend(_split_punctuation(token))
    return tokens


def _read_heredoc_delimiter(line: str, start: int) -> tuple[str, bool, int] | None:
    """Read a delimiter word and whether quote removal disables its expansions."""
    characters: list[str] = []
    quoted = False
    index = start
    while index < len(line):
        character = line[index]
        if character.isspace() or character in ";|&()<>":
            break
        if character == "\\":
            quoted = True
            if index + 1 >= len(line):
                return None
            characters.append(line[index + 1])
            index += 2
            continue
        if character in "'\"":
            quoted = True
            quote = character
            index += 1
            while index < len(line) and line[index] != quote:
                if quote == '"' and line[index] == "\\" and index + 1 < len(line):
                    characters.append(line[index + 1])
                    index += 2
                    continue
                characters.append(line[index])
                index += 1
            if index >= len(line):
                return None
            index += 1
            continue
        characters.append(character)
        index += 1
    return ("".join(characters), quoted, index) if characters else None


def _heredoc_delimiters(line: str) -> list[tuple[str, bool, bool]]:
    """Return (delimiter, strip-tabs, expands) for real shell here-doc redirects."""
    delimiters: list[tuple[str, bool, bool]] = []
    index = 0
    quote: str | None = None
    while index < len(line):
        character = line[index]
        if quote:
            if character == "\\" and quote == '"':
                index += 2
                continue
            if character == quote:
                quote = None
            index += 1
            continue
        if character in "'\"":
            quote = character
            index += 1
            continue
        if character == "\\":
            index += 2
            continue
        if character == "#" and (index == 0 or line[index - 1].isspace() or line[index - 1] in ";|&(){}"):
            break
        if line.startswith("<<<", index):
            index += 3
            continue
        if line.startswith("<<", index):
            index += 2
            strip_tabs = index < len(line) and line[index] == "-"
            if strip_tabs:
                index += 1
            while index < len(line) and line[index].isspace():
                index += 1
            result = _read_heredoc_delimiter(line, index)
            if result is None:
                return delimiters
            delimiter, quoted, index = result
            delimiters.append((delimiter, strip_tabs, not quoted))
            continue
        index += 1
    return delimiters


def _unquoted_heredoc_substitutions(body: str) -> list[str]:
    """Find substitutions Bash evaluates in an unquoted heredoc body.

    Quotes in an unquoted heredoc are data, so unlike normal shell source they
    do not protect a command substitution. Backslashes only suppress the
    expansions Bash recognizes in this context.
    """
    fragments: list[str] = []
    index = 0
    while index < len(body):
        character = body[index]
        if character == "\\" and index + 1 < len(body):
            if body[index + 1] in "$`\\\n":
                index += 2
                continue
            index += 1
            continue
        if body.startswith("$(", index):
            if body.startswith("$((", index):
                end = body.find("))", index + 3)
                index = len(body) if end < 0 else end + 2
                continue
            result = _read_parenthesized(body, index + 2)
            if result is None:
                raise ValueError("unterminated command substitution in heredoc")
            fragment, index = result
            fragments.append(fragment)
            continue
        if character == "`":
            result = _read_backticks(body, index + 1)
            if result is None:
                raise ValueError("unterminated backtick substitution in heredoc")
            fragment, index = result
            fragments.append(fragment)
            continue
        index += 1
    return fragments


def _without_heredoc_bodies(script: str) -> tuple[str, list[str]]:
    """Blank literal heredoc text and retain only expansions from unquoted bodies."""
    output: list[str] = []
    substitutions: list[str] = []
    pending: list[tuple[str, bool, bool]] = []
    body: list[str] = []
    for line in script.splitlines(keepends=True):
        if pending:
            delimiter, strip_tabs, expands = pending[0]
            candidate = line.rstrip("\r\n")
            if strip_tabs:
                candidate = candidate.lstrip("\t")
            if candidate == delimiter:
                if expands:
                    substitutions.extend(_unquoted_heredoc_substitutions("".join(body)))
                pending.pop(0)
                body = []
            elif expands:
                body.append(line)
            output.append("\n")
            continue
        output.append(line)
        pending.extend(_heredoc_delimiters(line))
    return "".join(output), substitutions


def _tokens_for_script(script: str) -> list[str]:
    script, _ = _without_heredoc_bodies(script)
    # An escaped newline is a continuation, not a command boundary. Remaining
    # newlines can be represented as semicolons for the command-position scan.
    script = script.replace("\\\r\n", "").replace("\\\n", "")
    return _lex_shell(script.replace("\r\n", "\n").replace("\n", ";"))


def _consume_parameter_and_arithmetic_expansions(tokens: list[str]) -> list[str]:
    """Keep variable names in ${...} and $((...)) out of command positions."""
    compacted: list[str] = []
    index = 0
    while index < len(tokens):
        if tokens[index] == "$" and index + 1 < len(tokens) and tokens[index + 1] == "{":
            depth = 1
            index += 2
            while index < len(tokens) and depth:
                if tokens[index] == "{":
                    depth += 1
                elif tokens[index] == "}":
                    depth -= 1
                index += 1
            compacted.append("$parameter")
            continue
        if tokens[index:index + 3] == ["$", "(", "("]:
            depth = 2
            index += 3
            while index < len(tokens) and depth:
                if tokens[index] == "(":
                    depth += 1
                elif tokens[index] == ")":
                    depth -= 1
                index += 1
            compacted.append("$arithmetic")
            continue
        compacted.append(tokens[index])
        index += 1
    return compacted


def _read_parenthesized(script: str, start: int) -> tuple[str, int] | None:
    """Read a $() or process-substitution body, honoring quotes and nesting."""
    depth = 1
    index = start
    quote: str | None = None
    while index < len(script):
        character = script[index]
        if quote:
            if character == "\\" and quote == '"':
                index += 2
                continue
            if character == quote:
                quote = None
            index += 1
            continue
        if character in "'\"":
            quote = character
        elif character == "\\":
            index += 2
            continue
        elif character == "#" and (index == 0 or script[index - 1].isspace() or script[index - 1] in ";|&(){}"):
            newline = script.find("\n", index)
            index = len(script) if newline < 0 else newline + 1
            continue
        elif character == "(":
            depth += 1
        elif character == ")":
            depth -= 1
            if depth == 0:
                return script[start:index], index + 1
        index += 1
    return None


def _read_backticks(script: str, start: int) -> tuple[str, int] | None:
    index = start
    content: list[str] = []
    while index < len(script):
        character = script[index]
        if character == "\\" and index + 1 < len(script):
            content.extend((character, script[index + 1]))
            index += 2
            continue
        if character == "`":
            return "".join(content), index + 1
        content.append(character)
        index += 1
    return None


def _command_substitutions(script: str) -> list[str]:
    """Find executable nested shell fragments, never quoted literals or comments."""
    fragments: list[str] = []
    index = 0
    quote: str | None = None
    while index < len(script):
        character = script[index]
        if quote:
            if character == "\\" and quote == '"':
                index += 2
                continue
            if quote == "'":
                if character == quote:
                    quote = None
                index += 1
                continue
            if character == "'":
                # Single quotes inside double quotes have no special meaning.
                index += 1
                continue
            if character == quote:
                quote = None
                index += 1
                continue
        if character in "'\"":
            quote = character
            index += 1
            continue
        if character == "\\":
            index += 2
            continue
        if character == "#" and (index == 0 or script[index - 1].isspace() or script[index - 1] in ";|&(){}"):
            newline = script.find("\n", index)
            index = len(script) if newline < 0 else newline + 1
            continue
        if script.startswith("$(", index):
            # $((...)) is arithmetic expansion, not a command invocation.
            if script.startswith("$((", index):
                end = script.find("))", index + 3)
                index = len(script) if end < 0 else end + 2
                continue
            result = _read_parenthesized(script, index + 2)
            if result is None:
                raise ValueError("unterminated command substitution")
            fragment, index = result
            fragments.append(fragment)
            continue
        if character in "<>" and index + 1 < len(script) and script[index + 1] == "(":
            result = _read_parenthesized(script, index + 2)
            if result is None:
                raise ValueError("unterminated process substitution")
            fragment, index = result
            fragments.append(fragment)
            continue
        if character == "`":
            result = _read_backticks(script, index + 1)
            if result is None:
                raise ValueError("unterminated backtick substitution")
            fragment, index = result
            fragments.append(fragment)
            continue
        index += 1
    return fragments


def _program_name(word: str) -> str:
    return word.rsplit("/", 1)[-1]


def _simple_command_argv(tokens: list[str], start: int) -> list[str]:
    """Return the words of one simple command, excluding redirect targets."""
    argv: list[str] = []
    index = start
    while index < len(tokens):
        token = tokens[index]
        if token in SEPARATORS or token in CLOSERS or token in {";;", ";&", ";;&"}:
            break
        if token.isdigit() and index + 1 < len(tokens) and tokens[index + 1] in REDIRECTIONS:
            index += 1
            continue
        if token in REDIRECTIONS:
            index += 2
            continue
        argv.append(token)
        index += 1
    return argv


def _after_options(args: list[str], options_with_values: frozenset[str] = frozenset()) -> list[str]:
    index = 0
    while index < len(args):
        argument = args[index]
        if argument == "--":
            return args[index + 1:]
        if argument in options_with_values:
            index += 2
            continue
        if any(argument.startswith(f"{option}=") for option in options_with_values):
            index += 1
            continue
        if argument.startswith("-") and argument != "-":
            index += 1
            continue
        return args[index:]
    return []


def _env_target(args: list[str]) -> tuple[list[str], list[str]]:
    """Return an env utility target and any -S split-string payloads."""
    split_strings: list[str] = []
    index = 0
    options_with_values = frozenset({"-u", "--unset", "-C", "--chdir", "-S", "--split-string"})
    while index < len(args):
        argument = args[index]
        if argument == "--":
            return args[index + 1:], split_strings
        if argument in {"-S", "--split-string"} and index + 1 < len(args):
            split_strings.append(args[index + 1])
            index += 2
            continue
        if argument in options_with_values:
            index += 2
            continue
        if any(argument.startswith(f"{option}=") for option in options_with_values):
            if argument.startswith("-S=") or argument.startswith("--split-string="):
                split_strings.append(argument.split("=", 1)[1])
            index += 1
            continue
        if argument.startswith("-") and argument != "-":
            index += 1
            continue
        if ASSIGNMENT.match(argument):
            index += 1
            continue
        return args[index:], split_strings
    return [], split_strings


def _shell_command_fragments(args: list[str]) -> list[str]:
    index = 0
    while index < len(args):
        argument = args[index]
        if argument in {"-c", "--command"} and index + 1 < len(args):
            return [args[index + 1]]
        if argument.startswith("-") and "c" in argument[1:] and index + 1 < len(args):
            return [args[index + 1]]
        if argument == "--":
            break
        index += 1
    return []


def _forbidden_in_argv(argv: list[str]) -> set[str]:
    while argv and ASSIGNMENT.match(argv[0]):
        argv = argv[1:]
    if not argv:
        return set()
    # A variable or command substitution at executable position can resolve to
    # a forbidden deployment binary after this static check has completed.
    # This deliberately fails closed instead of attempting shell evaluation.
    if "$" in argv[0] or "`" in argv[0]:
        return {DYNAMIC_EXECUTABLE}
    command = _program_name(argv[0])
    if command in FORBIDDEN_DEPLOYMENT_COMMANDS:
        return {command}
    args = argv[1:]
    if command == "env":
        target, split_strings = _env_target(args)
        found = _forbidden_in_argv(target)
        for split_string in split_strings:
            found.update(forbidden_deployment_commands(split_string))
        return found
    if command == "command":
        # command -v/-V only asks the shell to locate a name; it does not run it.
        if any(argument in {"-v", "-V"} for argument in args):
            return set()
        return _forbidden_in_argv(_after_options(args))
    if command == "sudo":
        return _forbidden_in_argv(_after_options(args, frozenset({"-C", "-D", "-g", "-h", "-p", "-r", "-R", "-t", "-T", "-u", "--chdir", "--close-from", "--group", "--host", "--prompt", "--role", "--type", "--user"})))
    if command in {"exec", "nice"}:
        return _forbidden_in_argv(_after_options(args, frozenset({"-a", "-n", "--adjustment"})))
    if command in {"time", "nohup", "setsid", "chronic"}:
        return _forbidden_in_argv(_after_options(args))
    if command == "timeout":
        target = _after_options(args, frozenset({"-k", "--kill-after", "-s", "--signal"}))
        return _forbidden_in_argv(target[1:]) if target else set()
    if command == "xargs":
        return _forbidden_in_argv(_after_options(args, frozenset({"-a", "-d", "-E", "-I", "-L", "-n", "-P", "-s", "--arg-file", "--delimiter", "--eof", "--max-args", "--max-lines", "--max-procs", "--max-chars", "--replace"})))
    if command in SHELLS:
        found: set[str] = set()
        for fragment in _shell_command_fragments(args):
            found.update(forbidden_deployment_commands(fragment))
        return found
    if command == "eval":
        return forbidden_deployment_commands(" ".join(args))
    if command == "find":
        for index, argument in enumerate(args):
            if argument in {"-exec", "-execdir", "-ok", "-okdir"}:
                return _forbidden_in_argv(args[index + 1:])
    return set()


def _command_argvs(tokens: list[str]) -> list[list[str]]:
    """Return statically visible executable command argument vectors."""
    commands: list[list[str]] = []
    expects_command = True
    mode: str | None = None
    skip_redirect_target = False
    index = 0
    while index < len(tokens):
        token = tokens[index]
        if mode == "case-body" and token in {";;", ";&", ";;&"}:
            mode = "case-pattern"
            expects_command = False
            index += 1
            continue
        if mode == "for-name":
            if token not in REDIRECTIONS and token not in SEPARATORS:
                mode = "for-list"
            index += 1
            continue
        if mode == "for-list":
            if token == ";":
                mode = None
                expects_command = True
            index += 1
            continue
        if mode == "case-subject":
            if token not in REDIRECTIONS and token not in SEPARATORS:
                mode = "case-await-in"
            index += 1
            continue
        if mode == "case-await-in":
            if token == "in":
                mode = "case-pattern"
            index += 1
            continue
        if mode == "case-pattern":
            if token == ")":
                mode = "case-body"
                expects_command = True
            index += 1
            continue
        if mode == "function-name":
            if token not in REDIRECTIONS and token not in SEPARATORS:
                mode = None
                expects_command = False
            index += 1
            continue
        if mode == "double-bracket":
            if token == "]]":
                mode = None
            index += 1
            continue
        if skip_redirect_target:
            if token not in REDIRECTIONS:
                skip_redirect_target = False
            index += 1
            continue
        if token in REDIRECTIONS:
            skip_redirect_target = True
            index += 1
            continue
        if token in {";;", ";&", ";;&"}:
            expects_command = True
            index += 1
            continue
        if token in SEPARATORS:
            expects_command = True
            index += 1
            continue
        if token in CLOSERS:
            expects_command = False
            index += 1
            continue
        if not expects_command:
            index += 1
            continue
        if token.isdigit() and index + 1 < len(tokens) and tokens[index + 1] in REDIRECTIONS:
            index += 1
            continue
        if token in {"!", "then", "elif", "else", "do", "while", "until"}:
            index += 1
            continue
        if token in {"fi", "done", "esac"}:
            if token == "esac":
                mode = None
            expects_command = False
            index += 1
            continue
        if token == "if":
            index += 1
            continue
        if token == "for":
            mode = "for-name"
            expects_command = False
            index += 1
            continue
        if token == "case":
            mode = "case-subject"
            expects_command = False
            index += 1
            continue
        if token == "function":
            mode = "function-name"
            expects_command = False
            index += 1
            continue
        if token == "[[":
            mode = "double-bracket"
            expects_command = False
            index += 1
            continue
        if index + 2 < len(tokens) and tokens[index + 1:index + 3] == ["(", ")"]:
            # A function definition such as kubectl() is not an invocation.
            expects_command = False
            index += 3
            continue
        commands.append(_simple_command_argv(tokens, index))
        expects_command = False
        index += 1
    return commands


def _forbidden_in_tokens(tokens: list[str]) -> set[str]:
    found: set[str] = set()
    for argv in _command_argvs(tokens):
        found.update(_forbidden_in_argv(argv))
    return found


def forbidden_deployment_commands(script: str) -> set[str]:
    """Return forbidden executables actually invoked by a shell run body."""
    source, heredoc_fragments = _without_heredoc_bodies(script)
    found = _forbidden_in_tokens(_consume_parameter_and_arithmetic_expansions(_tokens_for_script(source)))
    for fragment in [*heredoc_fragments, *_command_substitutions(source)]:
        found.update(forbidden_deployment_commands(fragment))
    return found


def _is_contract_make_argv(argv: list[str]) -> bool:
    """Recognize only the direct, executable contract Make invocation."""
    # An assignment can alter Make's included files, flags, path, or Python
    # subprocess behavior; a path-qualified executable can be repository or
    # runner-controlled code. The fixed contract permits only literal Make.
    return argv == ["make", "ci-eks-staging-contract"]


def script_runs_eks_staging_contract(script: str) -> bool:
    """Return whether a run step is the direct parsed CI target invocation.

    Comments, step names, quoted text passed to another command, and YAML
    metadata do not become an executable argv and therefore cannot satisfy the
    workflow contract.  The contract step is deliberately a single command so
    a shell conditional, wrapper, or failure-masking follow-up cannot turn an
    inert string into a passing gate.
    """
    source, heredoc_fragments = _without_heredoc_bodies(script)
    if heredoc_fragments or _command_substitutions(source):
        return False
    tokens = _consume_parameter_and_arithmetic_expansions(_tokens_for_script(source))
    commands = _command_argvs(tokens)
    return len(commands) == 1 and _is_contract_make_argv(commands[0])


def _decode_inline_yaml_scalar(value: str) -> str:
    value = value.strip()
    if not value or value[0] not in "'\"":
        return value
    quote = value[0]
    index = 1
    while index < len(value):
        if value[index] == quote:
            if quote == "'" and index + 1 < len(value) and value[index + 1] == quote:
                index += 2
                continue
            scalar = value[1:index]
            if value[index + 1 :].strip():
                raise ValueError("unexpected trailing YAML scalar content")
            return scalar.replace("''", "'") if quote == "'" else bytes(scalar, "utf-8").decode("unicode_escape")
        if quote == '"' and value[index] == "\\":
            index += 2
            continue
        index += 1
    raise ValueError("unterminated YAML scalar")


def _mapping_key(mapping_match: re.Match[str]) -> str:
    """Return a decoded bare, single-quoted, or double-quoted YAML key."""
    quoted_key = mapping_match.group("quoted_key")
    return _decode_inline_yaml_scalar(quoted_key) if quoted_key is not None else mapping_match.group("bare_key")


def _without_inline_yaml_comment(value: str) -> str:
    """Remove a YAML comment while preserving quoted scalar content."""
    quote: str | None = None
    index = 0
    while index < len(value):
        character = value[index]
        if quote:
            if character == "\\" and quote == '"':
                index += 2
                continue
            if character == quote:
                quote = None
            index += 1
            continue
        if character in "'\"":
            quote = character
        elif character == "#" and (index == 0 or value[index - 1].isspace()):
            return value[:index].rstrip()
        index += 1
    return value.rstrip()


def _contains_unquoted_yaml_flow_mapping(value: str) -> bool:
    """Return whether a YAML source line opens a flow-style mapping.

    The contract workflow deliberately supports only block-style mappings.  A
    regex scanner for block mappings cannot safely inspect a flow-style step
    such as ``- { run: kubectl apply ... }``; reject that syntax rather than
    silently skipping a potentially mutating step.  Quoted scalars and YAML
    comments are data, not flow mappings.
    """
    quote: str | None = None
    index = 0
    while index < len(value):
        character = value[index]
        if quote:
            if character == "\\" and quote == '"':
                index += 2
                continue
            if character == quote:
                quote = None
            index += 1
            continue
        if character in "'\"":
            quote = character
        elif character == "#" and (index == 0 or value[index - 1].isspace()):
            break
        elif character == "{":
            return True
        index += 1
    return False


def _block_scalar_end(lines: list[str], start: int, parent_indent: int) -> int:
    """Return the first line after a YAML block scalar body."""
    index = start + 1
    while index < len(lines):
        line = lines[index]
        indent = len(line) - len(line.lstrip(" "))
        if line.strip() and indent <= parent_indent:
            break
        index += 1
    return index


def _mapping_block_end(lines: list[str], start: int, parent_indent: int) -> int:
    """Return the first line after a block-mapping child scope."""
    index = start + 1
    while index < len(lines):
        line = lines[index]
        indent = len(line) - len(line.lstrip(" "))
        if line.strip() and indent <= parent_indent:
            break
        index += 1
    return index


def _workflow_mapping_entries(workflow: str) -> list[tuple[int, int, str, str]]:
    """Return mapping keys outside YAML block-scalar bodies."""
    lines = workflow.splitlines()
    entries: list[tuple[int, int, str, str]] = []
    index = 0
    while index < len(lines):
        mapping_match = YAML_MAPPING_KEY.match(lines[index])
        if mapping_match is None:
            index += 1
            continue
        indent = len(mapping_match.group("indent"))
        key = _mapping_key(mapping_match)
        value = mapping_match.group("value")
        entries.append((index + 1, indent, key, value))
        if BLOCK_SCALAR.match(value.strip()):
            index = _block_scalar_end(lines, index, indent)
            continue
        index += 1
    return entries


def workflow_flow_mapping_lines(workflow: str) -> list[int]:
    """Return flow-style mapping lines outside literal/folded scalar bodies.

    This narrow verifier does not implement a general YAML parser.  It does
    understand block scalar boundaries, so prose and shell examples inside a
    ``run: |`` body remain inert.  Elsewhere, every flow-map opener fails
    closed because it can encode a step that the block-mapping scanner would
    otherwise miss.
    """
    lines = workflow.splitlines()
    found: list[int] = []
    index = 0
    while index < len(lines):
        mapping_match = YAML_MAPPING_KEY.match(lines[index])
        if mapping_match is not None:
            indent = len(mapping_match.group("indent"))
            value = mapping_match.group("value")
            if BLOCK_SCALAR.match(value.strip()):
                index = _block_scalar_end(lines, index, indent)
                continue
        if _contains_unquoted_yaml_flow_mapping(lines[index]):
            found.append(index + 1)
        index += 1
    return found


def workflow_unsupported_yaml_syntax_lines(workflow: str) -> list[int]:
    """Return unsupported YAML node syntax outside block scalars.

    The contract intentionally accepts only simple block mappings and scalar
    list items. Explicit keys, tags, anchors, aliases, merges, document
    markers, directives, and flow sequences would add semantic surfaces that
    this narrow checker does not interpret. Reject them before comparing the
    source with the reviewed fixed workflow grammar.
    """
    lines = workflow.splitlines()
    found: list[int] = []
    index = 0
    while index < len(lines):
        mapping_match = YAML_MAPPING_KEY.match(lines[index])
        if mapping_match is not None:
            indent = len(mapping_match.group("indent"))
            value = mapping_match.group("value")
            if BLOCK_SCALAR.match(value.strip()):
                index = _block_scalar_end(lines, index, indent)
                continue
        source = _without_inline_yaml_comment(lines[index])
        unsupported = (
            YAML_EXPLICIT_MAPPING_KEY.match(source)
            or YAML_EXPLICIT_MAPPING_VALUE.match(source)
            or YAML_DOCUMENT_MARKER.match(source)
            or YAML_DIRECTIVE.match(source)
            or YAML_MERGE_KEY.match(source)
            or YAML_NODE_PREFIX.match(source)
            or YAML_FLOW_SEQUENCE.match(source)
        )
        if mapping_match is not None:
            inline_value = _without_inline_yaml_comment(mapping_match.group("value")).strip()
            unsupported = unsupported or inline_value.startswith(("!", "&", "*", "[", "{"))
        if unsupported:
            found.append(index + 1)
        index += 1
    return found


def _workflow_trigger_path_items(
    lines: list[str], trigger_line: int, trigger_indent: int, trigger: str
) -> tuple[str, ...]:
    """Read one trigger's block-style ``paths`` list without a YAML loader.

    The workflow contract intentionally supports only the simple, reviewable
    mapping/list form used in the checked-in Actions workflow.  A flow scalar,
    alias, nested value, or continuation could otherwise conceal an unreviewed
    source filter, so reject it instead of trying to interpret general YAML.
    """
    trigger_end = _mapping_block_end(lines, trigger_line, trigger_indent)
    path_mappings: list[tuple[int, int, str]] = []
    for index in range(trigger_line + 1, trigger_end):
        mapping_match = YAML_MAPPING_KEY.match(lines[index])
        if mapping_match is None:
            continue
        indent = len(mapping_match.group("indent"))
        if indent == trigger_indent + 2 and _mapping_key(mapping_match) == "paths":
            path_mappings.append((index, indent, mapping_match.group("value")))
    if len(path_mappings) != 1:
        raise ValueError(f"workflow {trigger} trigger must define exactly one paths list")

    paths_line, paths_indent, inline_value = path_mappings[0]
    if _without_inline_yaml_comment(inline_value).strip():
        raise ValueError(f"workflow {trigger} trigger paths must use a block list")
    paths_end = _mapping_block_end(lines, paths_line, paths_indent)
    items: list[str] = []
    for index in range(paths_line + 1, paths_end):
        line = lines[index]
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        indent = len(line) - len(line.lstrip(" "))
        if indent != paths_indent + 2 or not stripped.startswith("- "):
            raise ValueError(
                f"workflow {trigger} trigger paths must contain one scalar list item per line"
            )
        scalar = _without_inline_yaml_comment(stripped[2:]).strip()
        if not scalar or BLOCK_SCALAR.match(scalar):
            raise ValueError(f"workflow {trigger} trigger paths must contain non-empty scalar values")
        item = _decode_inline_yaml_scalar(scalar)
        if not item or item.startswith(("&", "*", "[", "{")):
            raise ValueError(f"workflow {trigger} trigger paths must contain plain path values")
        items.append(item)
    if not items:
        raise ValueError(f"workflow {trigger} trigger paths must not be empty")
    return tuple(items)


def workflow_trigger_paths(workflow: str) -> dict[str, tuple[str, ...]]:
    """Return the exact pull-request and push paths from the workflow source."""
    lines = workflow.splitlines()
    entries = _workflow_mapping_entries(workflow)
    on_mappings = [(line, indent, value) for line, indent, key, value in entries if key == "on" and indent == 0]
    if len(on_mappings) != 1:
        raise ValueError("workflow must define exactly one top-level on mapping")
    on_line, on_indent, on_value = on_mappings[0]
    if _without_inline_yaml_comment(on_value).strip():
        raise ValueError("workflow top-level on must use a block mapping")

    on_index = on_line - 1
    on_end = _mapping_block_end(lines, on_index, on_indent)
    trigger_mappings: dict[str, list[tuple[int, int, str]]] = {"pull_request": [], "push": []}
    for index in range(on_index + 1, on_end):
        mapping_match = YAML_MAPPING_KEY.match(lines[index])
        if mapping_match is None:
            continue
        indent = len(mapping_match.group("indent"))
        key = _mapping_key(mapping_match)
        if indent == on_indent + 2 and key in trigger_mappings:
            trigger_mappings[key].append((index, indent, mapping_match.group("value")))

    paths: dict[str, tuple[str, ...]] = {}
    for trigger, mappings in trigger_mappings.items():
        if len(mappings) != 1:
            raise ValueError(f"workflow on mapping must define exactly one {trigger} trigger")
        trigger_line, trigger_indent, inline_value = mappings[0]
        if _without_inline_yaml_comment(inline_value).strip():
            raise ValueError(f"workflow {trigger} trigger must use a block mapping")
        paths[trigger] = _workflow_trigger_path_items(lines, trigger_line, trigger_indent, trigger)
    return paths


def validate_workflow_path_filters(workflow: str) -> None:
    """Require both CI triggers to watch the complete reviewed contract input set."""
    expected = tuple(WORKFLOW_WATCHED_PATHS)
    if len(expected) != len(set(expected)):
        raise RuntimeError("workflow watched-path allowlist contains duplicate entries")
    actual = workflow_trigger_paths(workflow)
    for trigger in ("pull_request", "push"):
        if actual[trigger] != expected:
            missing = sorted(set(expected) - set(actual[trigger]))
            unexpected = sorted(set(actual[trigger]) - set(expected))
            duplicate = len(actual[trigger]) != len(set(actual[trigger]))
            details = []
            if missing:
                details.append("missing " + ", ".join(missing))
            if unexpected:
                details.append("unexpected " + ", ".join(unexpected))
            if duplicate:
                details.append("duplicate path entries")
            if not details:
                details.append("paths are not in the reviewed order")
            raise ValueError(
                f"workflow {trigger} trigger must watch exactly the reviewed contract inputs: "
                + "; ".join(details)
            )


def validate_workflow_permissions(workflow: str) -> None:
    """Require one non-overridable, least-privilege top-level permission map."""
    entries = _workflow_mapping_entries(workflow)
    permissions = [(line, indent, value) for line, indent, key, value in entries if key == "permissions"]
    top_level = [(line, value) for line, indent, value in permissions if indent == 0]
    if len(top_level) != 1:
        raise ValueError("workflow must define exactly one top-level permissions mapping")
    if any(indent != 0 for _, indent, _ in permissions):
        raise ValueError("workflow must not override top-level permissions in a job or step")
    line_number, inline_value = top_level[0]
    if _without_inline_yaml_comment(inline_value).strip():
        raise ValueError("workflow top-level permissions must use a block mapping")

    lines = workflow.splitlines()
    values: dict[str, str] = {}
    index = line_number
    while index < len(lines):
        line = lines[index]
        if not line.strip() or line.lstrip().startswith("#"):
            index += 1
            continue
        indent = len(line) - len(line.lstrip(" "))
        if indent == 0:
            break
        mapping_match = YAML_MAPPING_KEY.match(line)
        if mapping_match is None or mapping_match.group("indent") == "":
            raise ValueError("workflow top-level permissions must contain only scalar permission entries")
        key = _mapping_key(mapping_match)
        value = _without_inline_yaml_comment(mapping_match.group("value"))
        if not value.strip() or BLOCK_SCALAR.match(value.strip()):
            raise ValueError("workflow top-level permissions must contain only scalar permission entries")
        if key in values:
            raise ValueError("workflow top-level permissions must not repeat a permission")
        values[key] = _decode_inline_yaml_scalar(value)
        index += 1
    if values != {"contents": "read"}:
        raise ValueError("workflow top-level permissions must be exactly contents: read")


def validate_workflow_runner(workflow: str) -> None:
    """Require the single reviewed GitHub-hosted runner for this contract."""
    runners = [
        (line, _decode_inline_yaml_scalar(_without_inline_yaml_comment(value)))
        for line, _, key, value in _workflow_mapping_entries(workflow)
        if key == "runs-on"
    ]
    if len(runners) != 1 or runners[0][1] != REVIEWED_RUNNER:
        raise ValueError(
            f"workflow must use exactly one reviewed GitHub-hosted runner: {REVIEWED_RUNNER}"
        )


def validate_staging_oidc_trust_policy(trust: dict[str, object]) -> None:
    """Require one exact, narrow staging OIDC trust statement."""
    if trust.get("Version") != "2012-10-17":
        raise ValueError("OIDC trust policy must use Version 2012-10-17")
    statements = trust.get("Statement")
    if not isinstance(statements, list) or len(statements) != 1:
        raise ValueError("OIDC trust policy must contain exactly one staging OIDC statement")
    expected_principal = {"Federated": OIDC_FEDERATED_PRINCIPAL}
    expected_condition = {"StringEquals": {OIDC_AUDIENCE_CLAIM: OIDC_AUDIENCE, OIDC_SUBJECT_CLAIM: SUBJECT}}
    statement = statements[0]
    if not isinstance(statement, dict):
        raise ValueError("OIDC trust statement must be an object")
    if statement.get("Effect") != "Allow":
        raise ValueError("OIDC trust statement must allow only the approved staging identity")
    if statement.get("Principal") != expected_principal:
        raise ValueError("OIDC trust statement has an unapproved principal")
    if statement.get("Action") != "sts:AssumeRoleWithWebIdentity":
        raise ValueError("OIDC trust statement has an unapproved action")
    if statement.get("Condition") != expected_condition:
        raise ValueError("OIDC trust statement must require the exact audience and staging subject")


def workflow_action_uses(workflow: str) -> list[tuple[int, str]]:
    """Extract action references while excluding YAML block-scalar bodies.

    This contract is deliberately fail-closed: a `uses:` mapping outside a
    block scalar is treated as an action reference and must be allowlisted.
    """
    actions: list[tuple[int, str]] = []
    for line, _, key, value in _workflow_mapping_entries(workflow):
        if key == "uses":
            actions.append((line, _decode_inline_yaml_scalar(_without_inline_yaml_comment(value))))
    return actions


def workflow_action_violations(workflow: str) -> list[tuple[int, str]]:
    return [(line, action) for line, action in workflow_action_uses(workflow) if action not in ALLOWED_ACTIONS]


def workflow_execution_configuration_violations(workflow: str) -> list[tuple[int, str]]:
    """Return workflow mappings that can alter execution outside a run scalar.

    This contract workflow needs no environment, shell, container/service,
    checkout-input, lifecycle, or gate-control customization. Rejecting those
    controls keeps startup hooks such as ``BASH_ENV``, custom shell templates,
    container entrypoints, unreviewed checkout sources, alternate Makefile
    locations, GitHub environment context, and background/cancelled, skipped,
    or masked failures from bypassing the parsed sole ``run`` command.
    """
    return [
        (line, key)
        for line, _, key, _ in _workflow_mapping_entries(workflow)
        if key in FORBIDDEN_EXECUTION_CONFIGURATION_KEYS
    ]


def expected_contract_semantic_lines() -> tuple[str, ...]:
    """Return the reviewed fixed grammar for the non-deployment workflow."""
    pull_request_paths = tuple(f"      - {path}" for path in WORKFLOW_WATCHED_PATHS)
    push_paths = tuple(f"      - {path}" for path in WORKFLOW_WATCHED_PATHS)
    return (
        f"name: {WORKFLOW_NAME}",
        "on:",
        "  pull_request:",
        "    paths:",
        *pull_request_paths,
        "  push:",
        "    branches:",
        "      - main",
        "    paths:",
        *push_paths,
        "  workflow_dispatch:",
        "permissions:",
        "  contents: read",
        "jobs:",
        f"  {CONTRACT_JOB_NAME}:",
        f"    runs-on: {REVIEWED_RUNNER}",
        f"    timeout-minutes: {CONTRACT_TIMEOUT_MINUTES}",
        "    steps:",
        f"      - name: {CHECKOUT_STEP_NAME}",
        f"        uses: {PINNED_CHECKOUT}",
        f"      - name: {CONTRACT_STEP_NAME}",
        "        run: make ci-eks-staging-contract",
    )


def workflow_semantic_lines(workflow: str) -> tuple[str, ...]:
    """Normalize the limited plain block syntax accepted by this contract."""
    normalized: list[str] = []
    for line_number, line in enumerate(workflow.splitlines(), start=1):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        mapping_match = YAML_MAPPING_KEY.match(line)
        if mapping_match is not None:
            indent = mapping_match.group("indent")
            after_indent = line[len(indent) :]
            if after_indent.startswith("-") and not re.match(r"^-\s+", after_indent):
                raise ValueError(f"workflow list item must use a spaced hyphen at line {line_number}")
            list_prefix = "- " if re.match(r"^-\s+", after_indent) else ""
            key = _mapping_key(mapping_match)
            value = _without_inline_yaml_comment(mapping_match.group("value")).strip()
            if BLOCK_SCALAR.match(value):
                raise ValueError(f"workflow block scalars are unsupported at line {line_number}")
            scalar = _decode_inline_yaml_scalar(value) if value else ""
            normalized.append(f"{indent}{list_prefix}{key}:{' ' + scalar if scalar else ''}")
            continue
        list_match = YAML_LIST_ITEM.match(line)
        if list_match is not None:
            value = _without_inline_yaml_comment(list_match.group("value")).strip()
            if not value or BLOCK_SCALAR.match(value):
                raise ValueError(f"workflow list item must be a non-empty scalar at line {line_number}")
            normalized.append(
                f"{list_match.group('indent')}- {_decode_inline_yaml_scalar(value)}"
            )
            continue
        raise ValueError(f"workflow must use simple block mapping/list syntax at line {line_number}")
    return tuple(normalized)


def validate_canonical_workflow_structure(workflow: str) -> None:
    """Require the one reviewed workflow, job, and two-step execution grammar."""
    actual = workflow_semantic_lines(workflow)
    expected = expected_contract_semantic_lines()
    if actual == expected:
        return
    first_difference = next(
        (
            index + 1
            for index, (actual_line, expected_line) in enumerate(zip(actual, expected))
            if actual_line != expected_line
        ),
        min(len(actual), len(expected)) + 1,
    )
    raise ValueError(
        "workflow must match the reviewed canonical contract structure "
        f"(first difference at semantic line {first_difference})"
    )


def _inline_run_continuation_line(lines: list[str], start: int, parent_indent: int) -> int | None:
    """Return a forbidden continuation line after an inline ``run:`` scalar.

    Plain YAML scalars may continue onto a more-indented physical line.  This
    narrow workflow verifier intentionally does not reconstruct that grammar:
    a continuation could append a shell chain after the reviewed Make command.
    Step-level sibling mappings such as ``name:`` are still accepted; every
    other non-comment continuation fails closed.
    """
    index = start + 1
    while index < len(lines):
        line = lines[index]
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            index += 1
            continue
        indent = len(line) - len(line.lstrip(" "))
        if indent <= parent_indent:
            return None
        mapping_match = YAML_MAPPING_KEY.match(line)
        if mapping_match is not None and indent == parent_indent + 2:
            # A mapping at the list item's property indentation is a sibling
            # step field, not a continuation of the run scalar.
            return None
        return index + 1
    return None


def workflow_run_scripts(workflow: str) -> list[tuple[int, str]]:
    """Extract inline, literal, and folded GitHub Actions `run` scalars."""
    lines = workflow.splitlines()
    scripts: list[tuple[int, str]] = []
    index = 0
    while index < len(lines):
        mapping_match = YAML_MAPPING_KEY.match(lines[index])
        if mapping_match is None:
            index += 1
            continue
        parent_indent = len(mapping_match.group("indent"))
        value = mapping_match.group("value")
        if _mapping_key(mapping_match) != "run":
            if BLOCK_SCALAR.match(value.strip()):
                index = _block_scalar_end(lines, index, parent_indent)
                continue
            index += 1
            continue
        if not BLOCK_SCALAR.match(value.strip()):
            continuation_line = _inline_run_continuation_line(lines, index, parent_indent)
            if continuation_line is not None:
                raise ValueError(
                    "workflow inline run scalar must not use continuation lines; "
                    f"line {continuation_line} is unsupported"
                )
            scripts.append((index + 1, _decode_inline_yaml_scalar(value)))
            index += 1
            continue
        style = value.lstrip()[0]
        block: list[tuple[int, str]] = []
        index += 1
        while index < len(lines):
            line = lines[index]
            indent = len(line) - len(line.lstrip(" "))
            if line.strip() and indent <= parent_indent:
                break
            block.append((indent, line))
            index += 1
        nonblank_indents = [indent for indent, line in block if line.strip()]
        content_indent = min(nonblank_indents, default=parent_indent + 1)
        content = [(indent - content_indent, line[content_indent:] if line.strip() else "") for indent, line in block]
        if style == "|":
            script = "\n".join(line for _, line in content)
        else:
            script_parts: list[str] = []
            for content_index, (indent, line) in enumerate(content):
                if content_index:
                    previous_indent, previous_line = content[content_index - 1]
                    script_parts.append("\n" if not previous_line or not line or previous_indent > 0 or indent > 0 else " ")
                script_parts.append(line)
            script = "".join(script_parts)
        scripts.append((index - len(block) + 1, script))
    return scripts


def workflow_deployment_command_violations(workflow: str) -> list[tuple[int, set[str]]]:
    violations: list[tuple[int, set[str]]] = []
    for line, script in workflow_run_scripts(workflow):
        commands = forbidden_deployment_commands(script)
        if commands:
            violations.append((line, commands))
    return violations


def validate_staging_workflow_contract(workflow: str, makefile: str, trust: dict[str, object]) -> None:
    """Validate the pinned, read-only workflow and its sole OIDC trust rule."""
    unsupported_yaml_lines = workflow_unsupported_yaml_syntax_lines(workflow)
    if unsupported_yaml_lines:
        raise ValueError(
            "workflow must not use unsupported YAML node syntax outside block scalars; "
            + ", ".join(f"line {line}" for line in unsupported_yaml_lines)
        )
    flow_mapping_lines = workflow_flow_mapping_lines(workflow)
    if flow_mapping_lines:
        raise ValueError(
            "workflow must not use YAML flow-style mappings in this contract; "
            + ", ".join(f"line {line}" for line in flow_mapping_lines)
        )
    validate_workflow_permissions(workflow)
    validate_workflow_runner(workflow)
    execution_configuration_violations = workflow_execution_configuration_violations(workflow)
    if execution_configuration_violations:
        raise ValueError(
            "workflow must not configure execution-affecting environment, shell, container/service, checkout, lifecycle, or gate controls: "
            + ", ".join(f"line {line}: {key}" for line, key in execution_configuration_violations)
        )
    run_scripts = workflow_run_scripts(workflow)
    if len(run_scripts) != 1 or not script_runs_eks_staging_contract(run_scripts[0][1]):
        raise ValueError("workflow must have exactly one run step: make ci-eks-staging-contract")
    validate_canonical_workflow_structure(workflow)
    actions = workflow_action_uses(workflow)
    if PINNED_CHECKOUT not in {action for _, action in actions}:
        raise ValueError("workflow must use the approved pinned checkout action")
    action_violations = workflow_action_violations(workflow)
    if action_violations:
        raise ValueError(
            "workflow actions must use the explicit pinned allowlist: "
            + ", ".join(f"line {line}: {action}" for line, action in action_violations)
        )
    violations = workflow_deployment_command_violations(workflow)
    if violations:
        raise ValueError(
            "workflow run steps must not invoke aws, kubectl, kustomize, or a dynamically resolved executable: "
            + ", ".join(f"line {line}: {', '.join(sorted(commands))}" for line, commands in violations)
        )
    if "eks-apply-staging" in re.sub(r"^#.*$", "", workflow, flags=re.MULTILINE):
        raise ValueError("workflow must not invoke eks-apply-staging directly")
    if not re.search(r"^eks-apply-staging:\s+eks-supply-chain-validate\s*$", makefile, re.MULTILINE):
        raise ValueError("eks-apply-staging must require eks-supply-chain-validate")
    validate_staging_oidc_trust_policy(trust)


def main() -> int:
    try:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        validate_staging_workflow_contract(
            workflow,
            MAKEFILE.read_text(encoding="utf-8"),
            json.loads(TRUST.read_text(encoding="utf-8")),
        )
        validate_workflow_path_filters(workflow)
    except ValueError as exc:
        print(f"EKS staging workflow contract validation failed: {exc}", file=sys.stderr)
        return 2
    print("EKS staging workflow and OIDC trust-policy contract tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
