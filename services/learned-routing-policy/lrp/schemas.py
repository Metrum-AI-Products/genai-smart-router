"""Bounded public contract; caller identifiers and unknown inventory are discarded."""

from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

TargetKey = tuple[str, str]


class Model(BaseModel):
    model_config = ConfigDict(extra="ignore", allow_inf_nan=False, populate_by_name=True)


class Caller(Model):
    project: str = Field(default="", max_length=256)
    environment: str = Field(default="", max_length=256)


class Target(Model):
    provider: str = Field(min_length=1, max_length=256)
    model: str = Field(min_length=1, max_length=512)
    model_ref: str = Field(default="", alias="modelRef", max_length=512)
    dialect: str = Field(default="openai-chat", max_length=64)
    tier: str = Field(default="", max_length=64)
    weight: float = Field(default=1, ge=0)
    input_price: float | None = Field(default=None, alias="inputPricePerMillionUsd", ge=0)
    output_price: float | None = Field(default=None, alias="outputPricePerMillionUsd", ge=0)

    @property
    def key(self) -> TargetKey:
        return self.provider, self.model


class Payload(Model):
    group: str = Field(min_length=1, max_length=256)
    targets: list[Target] = Field(min_length=1, max_length=128)
    context: dict[str, Any] = Field(default_factory=dict)
    request: dict[str, Any] | None = None
    text: str | None = Field(default=None, max_length=1_100_000)
    caller: Caller = Field(default_factory=Caller)
    input_modalities: list[str] = Field(default_factory=list, alias="inputModalities")

    @field_validator("request")
    @classmethod
    def discard_raw_inventory(cls, value: dict[str, Any] | None) -> dict[str, Any] | None:
        if value is None:
            return None
        allowed = {
            "system",
            "messages",
            "input",
            "input_parts",
            "tools",
            "max_tokens",
            "temperature",
            "stream",
            "stop",
            "reasoning",
            "response_format",
        }
        return {key: item for key, item in value.items() if key in allowed}

    @model_validator(mode="after")
    def reject_ambiguous_skins(self) -> Payload:
        seen: dict[TargetKey, tuple[str, str]] = {}
        for target in self.targets:
            skin = target.dialect, target.model_ref
            if target.key in seen and seen[target.key] != skin:
                raise ValueError("ambiguous target identity across dialect or catalog aliases")
            seen[target.key] = skin
        return self


class GroupConfig(Model):
    quality_floor: float = Field(default=0.8, ge=0, le=1)
    explore_rate: float = Field(default=0, ge=0, le=1)
    pin_ttl_s: float = Field(default=0, ge=0, le=86400)
    min_train_rows: int = Field(default=200, ge=200)
    mode: Literal["enforce", "shadow"] = "enforce"
    # Explicit operator evidence can distinguish omitted known-free prices from unknown.
    zero_price_targets: list[tuple[str, str]] = Field(default_factory=list)
    exploration_projects: list[str] = Field(default_factory=list)


class ServiceConfig(Model):
    groups: dict[str, GroupConfig]
    max_pins: int = Field(default=10000, ge=1, le=100000)
    inference_workers: int = Field(default=2, ge=1, le=4)
    deadline_ms: int = Field(default=200, ge=1, le=200)


class RequestRow(Model):
    schema_version: Literal["lrp.request.v1"] = "lrp.request.v1"
    request_id: str
    captured_at: str
    source: str
    group: str
    dialect: str = "openai-chat"
    caller: Caller = Field(default_factory=Caller)
    session_key: str = ""
    turn_index: int = Field(default=0, ge=0)
    messages: list[dict[str, Any]] = Field(default_factory=list)
    system: str = ""
    input: str = ""
    tools: list[dict[str, Any]] = Field(default_factory=list)
    response_format: dict[str, Any] | None = None
    max_tokens: int | None = Field(default=None, ge=0)
    context: dict[str, Any] = Field(default_factory=dict)
    verifier: dict[str, Any] = Field(default_factory=lambda: {"kind": "none"})


class OfflineTarget(Model):
    provider: str
    model: str
    model_ref: str = ""


class Usage(Model):
    input_tokens: int = Field(ge=0)
    output_tokens: int = Field(ge=0)


class Pricing(Model):
    input_per_m_usd: float = Field(ge=0)
    output_per_m_usd: float = Field(ge=0)
    source: str
    fetched_at: str


class ResponseAttempt(Model):
    sequence: int = Field(ge=1)
    status: str
    error_class: str | None = None
    http_status: int | None = None
    duration_ms: float = Field(ge=0)
    ttfb_ms: float | None = Field(default=None, ge=0)
    usage: Usage | None = None
    cost_usd: float | None = Field(default=None, ge=0)
    billed_cost_usd: float | None = Field(default=None, ge=0)


class ResponseRow(Model):
    schema_version: Literal["lrp.response.v1"] = "lrp.response.v1"
    request_id: str
    target: OfflineTarget
    source: str
    started_at: str
    ttfb_ms: float | None = Field(default=None, ge=0)
    duration_ms: float = Field(ge=0)
    status: Literal["ok", "upstream_error", "timeout", "refused", "ineligible"]
    error_class: str | None = None
    content: str = ""
    tool_calls: list[dict[str, Any]] = Field(default_factory=list)
    finish_reason: str = ""
    usage: Usage | None = None
    pricing: Pricing | None = None
    cost_usd: float | None = Field(default=None, ge=0)
    billed_cost_usd: float | None = Field(default=None, ge=0)
    router_request_id: str | None = None
    serving_provider: str | None = None
    request_hash: str = ""
    config_hash: str = ""
    attempts: list[ResponseAttempt] = Field(default_factory=list)

    @model_validator(mode="after")
    def cost_matches(self) -> ResponseRow:
        if self.usage is not None and self.pricing is not None and self.cost_usd is not None:
            expected = (
                self.usage.input_tokens * self.pricing.input_per_m_usd
                + self.usage.output_tokens * self.pricing.output_per_m_usd
            ) / 1e6
            if abs(expected - self.cost_usd) > 1e-9:
                raise ValueError("stored cost does not match stored usage and pricing")
        return self


class JudgmentRow(Model):
    schema_version: Literal["lrp.judgment.v1"] = "lrp.judgment.v1"
    request_id: str
    target: OfflineTarget
    source: str
    method: str
    quality: float | None = Field(default=None, ge=0, le=1)
    detail: dict[str, Any] = Field(default_factory=dict)
    judge_model: str = ""
    judge_cost_usd: float | None = Field(default=None, ge=0)
    judged_at: str
    cache_key: str = ""
