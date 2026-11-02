export type Result<T, E = Error> =
  | { readonly success: true; readonly data: T; readonly error?: never }
  | { readonly success: false; readonly error: E; readonly data?: never };

export const ok = <T>(data: T): Result<T, never> => ({
  success: true,
  data
});

export const err = <E = Error>(error: E): Result<never, E> => ({
  success: false,
  error
});

export interface ResettableStatePort {
  reset(): void;
}

export abstract class BaseState implements ResettableStatePort {
  public isLoading = false;
  public hasError = false;
  public errorMessage: string | null = null;
  public lastTraceId: string | null = null;

  public abstract reset(): void;
}

export interface ApiPort<TInput, TOutput> {
  execute(input: TInput, traceId?: string): Promise<Result<TOutput, Error>>;
}

export abstract class BaseController<S extends BaseState> {
  protected readonly state: S;

  constructor(state: S) {
    this.state = state;
  }

  protected setPending(traceId?: string): void {
    this.state.isLoading = true;
    this.state.hasError = false;
    this.state.errorMessage = null;
    this.state.lastTraceId = traceId ?? null;
  }

  protected setFailure(error: Error, traceId?: string): void {
    this.state.isLoading = false;
    this.state.hasError = true;
    this.state.errorMessage = error.message;
    this.state.lastTraceId = traceId ?? null;
  }

  protected setSuccess(traceId?: string): void {
    this.state.isLoading = false;
    this.state.hasError = false;
    this.state.errorMessage = null;
    this.state.lastTraceId = traceId ?? null;
  }
}
