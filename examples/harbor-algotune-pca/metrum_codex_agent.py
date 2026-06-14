import json
import shlex
import tempfile
from pathlib import Path

from harbor.agents.installed.codex import Codex
from harbor.agents.installed.base import with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext
from harbor.models.trial.paths import EnvironmentPaths


class MetrumCodex(Codex):
    """Harbor Codex adapter variant that uses the router Responses provider."""

    @with_prompt_template
    async def run(
        self, instruction: str, environment: BaseEnvironment, context: AgentContext
    ) -> None:
        escaped_instruction = shlex.quote(instruction)

        if not self.model_name:
            raise ValueError("Model name is required")

        model = self.model_name.split("/")[-1]
        cli_flags = self.build_cli_flags()
        cli_flags_arg = (cli_flags + " ") if cli_flags else ""

        auth_json_path = self._resolve_auth_json_path()
        remote_codex_home = self._REMOTE_CODEX_HOME.as_posix()
        remote_secrets_dir = self._REMOTE_CODEX_SECRETS_DIR.as_posix()
        remote_auth_path = (self._REMOTE_CODEX_SECRETS_DIR / "auth.json").as_posix()
        remote_config_path = (self._REMOTE_CODEX_SECRETS_DIR / "config.toml").as_posix()
        router_key = self._get_env("METRUM_ROUTER_KEY") or ""
        router_base_url = (
            self._get_env("METRUM_ROUTER_BASE_URL")
            or self._get_env("OPENAI_BASE_URL")
            or ""
        )

        env: dict[str, str] = {
            "CODEX_HOME": remote_codex_home,
            "METRUM_ROUTER_KEY": router_key,
        }

        await self.exec_as_agent(
            environment,
            command=(
                f'mkdir -p "$CODEX_HOME" {shlex.quote(remote_secrets_dir)} '
                f"{shlex.quote(EnvironmentPaths.agent_dir.as_posix())}"
            ),
            env=env,
        )

        if auth_json_path:
            self.logger.debug("Codex auth: using auth.json from %s", auth_json_path)
            await environment.upload_file(auth_json_path, remote_auth_path)
        else:
            with tempfile.NamedTemporaryFile("w", delete=False) as tmp:
                json.dump({"METRUM_ROUTER_KEY": router_key}, tmp, indent=2)
                tmp.write("\n")
                auth_json_path = tmp.name
            try:
                await environment.upload_file(auth_json_path, remote_auth_path)
            finally:
                Path(auth_json_path).unlink(missing_ok=True)

        config_text = (
            f'model = "{model}"\n'
            'model_provider = "metrum-router"\n'
            '\n[model_providers."metrum-router"]\n'
            'name = "Metrum Router"\n'
            f'base_url = "{router_base_url}"\n'
            'env_key = "METRUM_ROUTER_KEY"\n'
            'wire_api = "responses"\n'
        )
        with tempfile.NamedTemporaryFile("w", delete=False) as tmp:
            tmp.write(config_text)
            config_path = tmp.name
        try:
            await environment.upload_file(config_path, remote_config_path)
        finally:
            Path(config_path).unlink(missing_ok=True)

        if environment.default_user is not None:
            await self.exec_as_root(
                environment,
                command=(
                    f"chown {environment.default_user} "
                    f"{shlex.quote(remote_auth_path)} {shlex.quote(remote_config_path)}"
                ),
            )

        setup_command = (
            f'ln -sf {shlex.quote(remote_auth_path)} "$CODEX_HOME/auth.json"\n'
            f'ln -sf {shlex.quote(remote_config_path)} "$CODEX_HOME/config.toml"\n'
        )

        skills_command = self._build_register_skills_command()
        if skills_command:
            setup_command += f"\n{skills_command}"

        mcp_command = self._build_register_mcp_servers_command()
        if mcp_command:
            setup_command += f"\n{mcp_command}"

        await self.exec_as_agent(environment, command=setup_command, env=env)

        try:
            await self.exec_as_agent(
                environment,
                command=(
                    "if [ -s ~/.nvm/nvm.sh ]; then . ~/.nvm/nvm.sh; fi; "
                    "codex exec "
                    "--dangerously-bypass-approvals-and-sandbox "
                    "--skip-git-repo-check "
                    f"--model {shlex.quote(model)} "
                    "--json "
                    "--enable unified_exec "
                    f"{cli_flags_arg}"
                    "-- "
                    f"{escaped_instruction} "
                    f"2>&1 </dev/null | tee "
                    f"{EnvironmentPaths.agent_dir / self._OUTPUT_FILENAME}"
                ),
                env=env,
            )
        finally:
            try:
                await self.exec_as_agent(
                    environment,
                    command=(
                        f"mkdir -p {EnvironmentPaths.agent_dir.as_posix()}\n"
                        'if [ -d "$CODEX_HOME/sessions" ]; then\n'
                        f"  rm -rf "
                        f"{(EnvironmentPaths.agent_dir / 'sessions').as_posix()}\n"
                        f'  cp -R "$CODEX_HOME/sessions" '
                        f'{(EnvironmentPaths.agent_dir / "sessions").as_posix()}\n'
                        "fi"
                    ),
                    env=env,
                )
            except Exception:
                self.logger.exception("Failed to copy Codex sessions")
