package com.cubevlmu.lodestar.update;

/** Base error for update checks, signature verification and downloads. */
public class LodestarUpdateException extends RuntimeException {

    public LodestarUpdateException(String message) {
        super(message);
    }

    public LodestarUpdateException(String message, Throwable cause) {
        super(message, cause);
    }
}
