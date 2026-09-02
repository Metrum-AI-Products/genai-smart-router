// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import { cn } from "@/lib/utils";

export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "outline" | "ghost";
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        "inline-flex h-9 items-center justify-center whitespace-nowrap rounded-md px-3 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-metrum-blue disabled:pointer-events-none disabled:opacity-50",
        variant === "default" && "border border-metrum-purple bg-metrum-purple text-white hover:bg-metrum-magenta",
        variant === "outline" && "border border-white/15 bg-white/[0.04] text-white hover:bg-white/[0.08]",
        variant === "ghost" && "text-white hover:bg-white/[0.08]",
        className,
      )}
      {...props}
    />
  ),
);
Button.displayName = "Button";
