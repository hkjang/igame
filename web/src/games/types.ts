export interface BuiltinGameProps {
  onStart: (metadata?: Record<string, unknown>) => Promise<boolean>;
  onFinish: (score: number, metadata?: Record<string, unknown>) => Promise<void>;
}
