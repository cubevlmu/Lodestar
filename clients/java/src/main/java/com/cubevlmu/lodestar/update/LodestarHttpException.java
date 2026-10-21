package com.cubevlmu.lodestar.update;

/** Non-2xx response from the server. */
public class LodestarHttpException extends LodestarUpdateException {

    private final int statusCode;
    private final String body;

    public LodestarHttpException(String message, int statusCode, String body) {
        super(message);
        this.statusCode = statusCode;
        this.body = body;
    }

    public int statusCode() {
        return statusCode;
    }

    public String body() {
        return body;
    }
}
