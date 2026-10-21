using System;

namespace Lodestar.Update;

/// <summary>Base error for update checks, signature verification and downloads.</summary>
public class LodestarUpdateException : Exception
{
    public LodestarUpdateException(string message) : base(message) { }

    public LodestarUpdateException(string message, Exception inner) : base(message, inner) { }
}

/// <summary>Non-2xx response from the server.</summary>
public sealed class LodestarHttpException : LodestarUpdateException
{
    public LodestarHttpException(string message, int statusCode, string? body) : base(message)
    {
        StatusCode = statusCode;
        Body = body;
    }

    public int StatusCode { get; }

    public string? Body { get; }
}
