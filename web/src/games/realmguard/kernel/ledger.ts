import type { TargetingMode } from "../types";

/**
 * The battle ledger is the complete player input record. Everything else about
 * a battle — kills, leaks, gold, the final score — is derived from replaying it
 * against published content, so this is the only thing a client is trusted to
 * report.
 */
export type KernelAction =
  | { op: "wave" }
  | { op: "build"; spot: string; tower: string; profile?: string }
  | { op: "upgrade"; spot: string; branch?: string }
  | { op: "sell"; spot: string }
  | { op: "target"; spot: string; mode: TargetingMode }
  | { op: "skill"; skill: string }
  | { op: "meteor"; x: number; y: number }
  | { op: "reinforce"; x: number; y: number }
  | { op: "hero"; x: number; y: number }
  | { op: "economy"; gold: number; lives: number }
  | { op: "defeat" };

export type KernelCommand = KernelAction & { tick: number };

export const KERNEL_LEDGER_LIMIT = 6000;
export const KERNEL_TICK_LIMIT = 288_000;

export interface KernelLedger {
  /** Bumped whenever the rules change in a way that invalidates old replays. */
  rules_version: string;
  config_digest: string;
  ticks: number;
  commands: KernelCommand[];
}

export const KERNEL_RULES_VERSION = "realmguard-kernel-1";

export class LedgerRecorder {
  private readonly commands: KernelCommand[] = [];
  private overflowed = false;

  record(command: KernelCommand) {
    if (this.commands.length >= KERNEL_LEDGER_LIMIT) {
      this.overflowed = true;
      return;
    }
    this.commands.push(command);
  }

  /** A truncated ledger can never replay, so the battle is marked unverifiable. */
  get truncated() {
    return this.overflowed;
  }

  build(configDigest: string, ticks: number): KernelLedger {
    return {
      rules_version: KERNEL_RULES_VERSION,
      config_digest: configDigest,
      ticks,
      commands: [...this.commands],
    };
  }
}
