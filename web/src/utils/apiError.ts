import { t } from "../i18n";

// 后端错误码 -> i18n key。未收录的错误码（如 db_error、create_failed）不直接展示给用户，
// 由调用方给出本地化的通用兜底文案。
const ERROR_MESSAGE_KEYS: Record<string, string> = {
  too_many_keys: "portal.errors.tooManyKeys",
  missing_keys: "portal.errors.missingKeys",
  payload_too_large: "portal.errors.payloadTooLarge",
  forbidden: "portal.errors.forbidden",
  rate_limited: "donate.rateLimited",
  alias_taken: "donate.aliasTaken",
};

export function apiErrorMessage(err: any, fallback: string): string {
  const code = err?.response?.data?.error;
  const key = typeof code === "string" ? ERROR_MESSAGE_KEYS[code] : undefined;
  return key ? t(key) : fallback;
}
