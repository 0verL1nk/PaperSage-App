export type ApiEnvelope<TData> = {
  code: number
  data: TData
  info: string
}

export type ApiEnvelopeInfo = Pick<ApiEnvelope<unknown>, 'code' | 'info'>

export const API_SUCCESS_CODE = 200
