export interface BuiltinGameProps {
  onStart: (metadata?: Record<string, unknown>) => Promise<boolean>;
  onFinish: (score: number, metadata?: Record<string, unknown>) => Promise<void>;
  onTelemetry?: (event: string, data?: Record<string, unknown>) => Promise<void>;
  onAuthoritativeComplete?: (payload: unknown) => Promise<unknown>;
  isRecording?: () => boolean;
  online?: boolean;
}
