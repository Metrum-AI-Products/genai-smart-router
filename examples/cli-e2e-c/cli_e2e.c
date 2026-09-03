// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

#define _POSIX_C_SOURCE 200809L

#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define OUT_CAP 65536
#define CMD_CAP 8192

static const char *env_or(const char *name, const char *fallback) {
    const char *value = getenv(name);
    return value && value[0] ? value : fallback;
}

static char *shell_quote(const char *s) {
    size_t len = 2;
    for (const char *p = s; *p; p++) {
        len += (*p == '\'') ? 4 : 1;
    }
    char *out = calloc(len + 1, 1);
    if (!out) {
        return NULL;
    }
    char *w = out;
    *w++ = '\'';
    for (const char *p = s; *p; p++) {
        if (*p == '\'') {
            memcpy(w, "'\\''", 4);
            w += 4;
        } else {
            *w++ = *p;
        }
    }
    *w++ = '\'';
    *w = 0;
    return out;
}

static int run_command(const char *cmd, char *out, size_t out_cap) {
    FILE *pipe = popen(cmd, "r");
    if (!pipe) {
        return 127;
    }
    size_t used = 0;
    while (used + 1 < out_cap) {
        size_t n = fread(out + used, 1, out_cap - used - 1, pipe);
        used += n;
        if (n == 0) {
            break;
        }
    }
    out[used] = 0;
    return pclose(pipe);
}

static int line_equals_expected(const char *output, const char *expected) {
    size_t expected_len = strlen(expected);
    const char *p = output;
    while (*p) {
        while (*p == '\r' || *p == '\n') {
            p++;
        }
        const char *start = p;
        while (*p && *p != '\r' && *p != '\n') {
            p++;
        }
        const char *end = p;
        while (start < end && isspace((unsigned char)*start)) {
            start++;
        }
        while (end > start && isspace((unsigned char)*(end - 1))) {
            end--;
        }
        if ((size_t)(end - start) == expected_len && memcmp(start, expected, expected_len) == 0) {
            return 1;
        }
    }
    return 0;
}

static int test_claude(const char *expect) {
    const char *base_url = env_or("ROUTER_BASE_URL", "http://127.0.0.1:8080");
    const char *token = env_or("ROUTER_TOKEN", "");
    const char *model = env_or("ROUTER_MODEL", "default");
    const char *bin = env_or("CLAUDE_BIN", "claude");
    const char *timeout = env_or("CLI_TIMEOUT_SECONDS", "120");
    char prompt[256];
    snprintf(prompt, sizeof(prompt), "Reply with exactly: %s", expect);

    char *q_base = shell_quote(base_url);
    char *q_token = shell_quote(token);
    char *q_model = shell_quote(model);
    char *q_bin = shell_quote(bin);
    char *q_prompt = shell_quote(prompt);
    if (!q_base || !q_token || !q_model || !q_bin || !q_prompt) {
        fprintf(stderr, "allocation failure\n");
        return 1;
    }

    char cmd[CMD_CAP];
    snprintf(cmd, sizeof(cmd),
             "env -u ANTHROPIC_API_KEY ANTHROPIC_BASE_URL=%s ANTHROPIC_AUTH_TOKEN=%s ANTHROPIC_MODEL=%s timeout %s %s --bare --print --model %s %s 2>&1",
             q_base, q_token, q_model, timeout, q_bin, q_model, q_prompt);
    free(q_base);
    free(q_token);
    free(q_model);
    free(q_bin);
    free(q_prompt);

    char output[OUT_CAP];
    int status = run_command(cmd, output, sizeof(output));
    if (status != 0 || !line_equals_expected(output, expect)) {
        fprintf(stderr, "claude failed: status=%d\n%s\n", status, output);
        return 1;
    }
    printf("claude: ok\n");
    return 0;
}

static int test_codex(const char *expect) {
    const char *base_url = env_or("ROUTER_BASE_URL", "http://127.0.0.1:8080");
    const char *token = env_or("ROUTER_TOKEN", "");
    const char *model = env_or("ROUTER_MODEL", "default");
    const char *bin = env_or("CODEX_BIN", "codex");
    const char *timeout = env_or("CLI_TIMEOUT_SECONDS", "180");
    const char *wire_api = env_or("CODEX_WIRE_API", "responses");
    char prompt[256];
    char base_v1[512];
    snprintf(prompt, sizeof(prompt), "Reply with exactly: %s", expect);
    snprintf(base_v1, sizeof(base_v1), "%s/v1", base_url);

    char *q_token = shell_quote(token);
    char *q_bin = shell_quote(bin);
    char *q_model_cfg = shell_quote("model=\"default\"");
    char *q_provider_cfg = shell_quote("model_provider=\"metrum-router\"");
    char base_cfg[700];
    snprintf(base_cfg, sizeof(base_cfg), "model_providers.metrum-router.base_url=\"%s\"", base_v1);
    char *q_base_cfg = shell_quote(base_cfg);
    char *q_name_cfg = shell_quote("model_providers.metrum-router.name=\"Metrum Router\"");
    char *q_key_cfg = shell_quote("model_providers.metrum-router.env_key=\"METRUM_ROUTER_KEY\"");
    char wire_cfg[128];
    snprintf(wire_cfg, sizeof(wire_cfg), "model_providers.metrum-router.wire_api=\"%s\"", wire_api);
    char *q_wire_cfg = shell_quote(wire_cfg);
    char model_cfg[256];
    snprintf(model_cfg, sizeof(model_cfg), "model=\"%s\"", model);
    free(q_model_cfg);
    q_model_cfg = shell_quote(model_cfg);
    char *q_prompt = shell_quote(prompt);
    if (!q_token || !q_bin || !q_model_cfg || !q_provider_cfg || !q_base_cfg || !q_name_cfg || !q_key_cfg || !q_wire_cfg || !q_prompt) {
        fprintf(stderr, "allocation failure\n");
        return 1;
    }

    char cmd[CMD_CAP];
    snprintf(cmd, sizeof(cmd),
             "METRUM_ROUTER_KEY=%s timeout %s %s exec --ignore-user-config --ephemeral "
             "-c %s -c %s -c %s -c %s -c %s -c %s %s 2>&1",
             q_token, timeout, q_bin, q_model_cfg, q_provider_cfg, q_name_cfg, q_base_cfg, q_key_cfg, q_wire_cfg, q_prompt);

    free(q_token);
    free(q_bin);
    free(q_model_cfg);
    free(q_provider_cfg);
    free(q_base_cfg);
    free(q_name_cfg);
    free(q_key_cfg);
    free(q_wire_cfg);
    free(q_prompt);

    char output[OUT_CAP];
    int status = run_command(cmd, output, sizeof(output));
    if (status != 0 || !line_equals_expected(output, expect)) {
        fprintf(stderr, "codex failed: status=%d\n%s\n", status, output);
        return 1;
    }
    printf("codex: ok\n");
    return 0;
}

static void usage(const char *argv0) {
    fprintf(stderr, "usage: %s [--tool claude|codex|both] [--expect text]\n", argv0);
}

int main(int argc, char **argv) {
    const char *tool = "both";
    const char *expect = "hello world";
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--tool") == 0 && i + 1 < argc) {
            tool = argv[++i];
        } else if (strcmp(argv[i], "--expect") == 0 && i + 1 < argc) {
            expect = argv[++i];
        } else {
            usage(argv[0]);
            return 2;
        }
    }

    int failed = 0;
    if (strcmp(tool, "claude") == 0 || strcmp(tool, "both") == 0) {
        failed |= test_claude(expect);
    }
    if (strcmp(tool, "codex") == 0 || strcmp(tool, "both") == 0) {
        failed |= test_codex(expect);
    }
    if (strcmp(tool, "claude") != 0 && strcmp(tool, "codex") != 0 && strcmp(tool, "both") != 0) {
        usage(argv[0]);
        return 2;
    }
    return failed ? 1 : 0;
}
