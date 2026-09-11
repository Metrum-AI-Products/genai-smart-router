# HARBOR-16: two concurrent callers with identical prompts but distinct secret nonces.
Each artifact must contain only its own nonce; cross-talk fails.
