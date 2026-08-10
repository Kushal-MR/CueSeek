package dev.cueseek.core.model

/**
 * The result of one call to an agent.
 *
 * Failures are values rather than exceptions. A generated client would throw on every
 * non-2xx response, which makes "the token was rejected" and "the JSON was malformed"
 * arrive through the same channel as a programming error, and makes forgetting to handle
 * either of them the path of least resistance (ADR-0009).
 */
sealed interface ApiResult<out T> {

    data class Success<out T>(val value: T) : ApiResult<T>

    data class Failure(val error: ApiError) : ApiResult<Nothing>

    val isSuccess: Boolean get() = this is Success

    fun getOrNull(): T? = (this as? Success)?.value

    fun errorOrNull(): ApiError? = (this as? Failure)?.error
}

inline fun <T, R> ApiResult<T>.map(transform: (T) -> R): ApiResult<R> = when (this) {
    is ApiResult.Success -> ApiResult.Success(transform(value))
    is ApiResult.Failure -> this
}

inline fun <T, R> ApiResult<T>.fold(
    onSuccess: (T) -> R,
    onFailure: (ApiError) -> R,
): R = when (this) {
    is ApiResult.Success -> onSuccess(value)
    is ApiResult.Failure -> onFailure(error)
}
