package com.cubevlmu.lodestar.update;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * A tiny JSON reader/writer.
 *
 * <p>The client deliberately has no third-party dependencies: dropping a jar
 * into an Android or desktop app just for an update check is not worth it, and
 * the response shape is small and fixed.
 */
final class Json {

    private final String text;
    private int pos;

    private Json(String text) {
        this.text = text;
    }

    static Object parse(String text) {
        Json parser = new Json(text);
        parser.skipWhitespace();
        Object value = parser.readValue();
        parser.skipWhitespace();
        if (parser.pos != text.length()) {
            throw new LodestarUpdateException("Trailing data in JSON response");
        }
        return value;
    }

    @SuppressWarnings("unchecked")
    static Map<String, Object> parseObject(String text) {
        Object value = parse(text);
        if (!(value instanceof Map)) {
            throw new LodestarUpdateException("Expected a JSON object");
        }
        return (Map<String, Object>) value;
    }

    /** Serialises a flat map of primitives, which is all a check request needs. */
    static String write(Map<String, ?> values) {
        StringBuilder builder = new StringBuilder("{");
        boolean first = true;
        for (Map.Entry<String, ?> entry : values.entrySet()) {
            Object value = entry.getValue();
            if (value == null) {
                continue;
            }
            if (!first) {
                builder.append(',');
            }
            first = false;
            writeString(builder, entry.getKey());
            builder.append(':');
            if (value instanceof Number || value instanceof Boolean) {
                builder.append(value);
            } else {
                writeString(builder, String.valueOf(value));
            }
        }
        return builder.append('}').toString();
    }

    private static void writeString(StringBuilder builder, String value) {
        builder.append('"');
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            switch (c) {
                case '"' -> builder.append("\\\"");
                case '\\' -> builder.append("\\\\");
                case '\n' -> builder.append("\\n");
                case '\r' -> builder.append("\\r");
                case '\t' -> builder.append("\\t");
                default -> {
                    if (c < 0x20) {
                        builder.append(String.format("\\u%04x", (int) c));
                    } else {
                        builder.append(c);
                    }
                }
            }
        }
        builder.append('"');
    }

    private Object readValue() {
        if (pos >= text.length()) {
            throw new LodestarUpdateException("Unexpected end of JSON");
        }
        char c = text.charAt(pos);
        return switch (c) {
            case '{' -> readObject();
            case '[' -> readArray();
            case '"' -> readString();
            case 't', 'f' -> readBoolean();
            case 'n' -> readNull();
            default -> readNumber();
        };
    }

    private Map<String, Object> readObject() {
        Map<String, Object> map = new LinkedHashMap<>();
        expect('{');
        skipWhitespace();
        if (peek() == '}') {
            pos++;
            return map;
        }
        while (true) {
            skipWhitespace();
            String key = readString();
            skipWhitespace();
            expect(':');
            skipWhitespace();
            map.put(key, readValue());
            skipWhitespace();
            char c = next();
            if (c == '}') {
                return map;
            }
            if (c != ',') {
                throw new LodestarUpdateException("Expected , or } in JSON object");
            }
        }
    }

    private List<Object> readArray() {
        List<Object> list = new ArrayList<>();
        expect('[');
        skipWhitespace();
        if (peek() == ']') {
            pos++;
            return list;
        }
        while (true) {
            skipWhitespace();
            list.add(readValue());
            skipWhitespace();
            char c = next();
            if (c == ']') {
                return list;
            }
            if (c != ',') {
                throw new LodestarUpdateException("Expected , or ] in JSON array");
            }
        }
    }

    private String readString() {
        expect('"');
        StringBuilder builder = new StringBuilder();
        while (true) {
            char c = next();
            if (c == '"') {
                return builder.toString();
            }
            if (c != '\\') {
                builder.append(c);
                continue;
            }
            char escaped = next();
            switch (escaped) {
                case '"' -> builder.append('"');
                case '\\' -> builder.append('\\');
                case '/' -> builder.append('/');
                case 'b' -> builder.append('\b');
                case 'f' -> builder.append('\f');
                case 'n' -> builder.append('\n');
                case 'r' -> builder.append('\r');
                case 't' -> builder.append('\t');
                case 'u' -> {
                    String hex = text.substring(pos, pos + 4);
                    pos += 4;
                    builder.append((char) Integer.parseInt(hex, 16));
                }
                default -> throw new LodestarUpdateException("Bad escape in JSON string: \\" + escaped);
            }
        }
    }

    private Object readNumber() {
        int start = pos;
        while (pos < text.length() && "-+.eE0123456789".indexOf(text.charAt(pos)) >= 0) {
            pos++;
        }
        String raw = text.substring(start, pos);
        if (raw.isEmpty()) {
            throw new LodestarUpdateException("Unexpected character in JSON: " + text.charAt(start));
        }
        if (raw.contains(".") || raw.contains("e") || raw.contains("E")) {
            return Double.parseDouble(raw);
        }
        return Long.parseLong(raw);
    }

    private Boolean readBoolean() {
        if (text.startsWith("true", pos)) {
            pos += 4;
            return Boolean.TRUE;
        }
        if (text.startsWith("false", pos)) {
            pos += 5;
            return Boolean.FALSE;
        }
        throw new LodestarUpdateException("Bad JSON literal");
    }

    private Object readNull() {
        if (text.startsWith("null", pos)) {
            pos += 4;
            return null;
        }
        throw new LodestarUpdateException("Bad JSON literal");
    }

    private void skipWhitespace() {
        while (pos < text.length() && Character.isWhitespace(text.charAt(pos))) {
            pos++;
        }
    }

    private char peek() {
        return pos < text.length() ? text.charAt(pos) : '\0';
    }

    private char next() {
        if (pos >= text.length()) {
            throw new LodestarUpdateException("Unexpected end of JSON");
        }
        return text.charAt(pos++);
    }

    private void expect(char expected) {
        char c = next();
        if (c != expected) {
            throw new LodestarUpdateException("Expected " + expected + " but found " + c);
        }
    }
}
