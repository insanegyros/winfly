<?php

declare(strict_types=1);

/**
 * An error whose message is shown to the administrator, with the HTTP status
 * and machine-readable code the API answers with.
 */
final class ApiError extends RuntimeException
{
    private int $httpStatus;
    private string $errorCode;

    public function __construct(string $message, int $httpStatus = 422, string $errorCode = 'INVALID_REQUEST')
    {
        parent::__construct($message);
        $this->httpStatus = $httpStatus;
        $this->errorCode = $errorCode;
    }

    public static function notFound(string $message): self
    {
        return new self($message, 404, 'NOT_FOUND');
    }

    public static function conflict(string $message, string $errorCode): self
    {
        return new self($message, 409, $errorCode);
    }

    public function httpStatus(): int
    {
        return $this->httpStatus;
    }

    public function errorCode(): string
    {
        return $this->errorCode;
    }
}
