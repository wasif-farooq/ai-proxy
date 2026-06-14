#!/usr/bin/env node

/**
 * consumer-node.mjs — AI Proxy consumer example (Node.js)
 *
 * Demonstrates how to authenticate with the proxy using the encrypted X-Auth
 * header, which is the recommended auth method for public APIs.
 *
 * Prerequisites:
 *   - Your client credentials (client_id + encryption_key) from the admin panel
 *   - Node.js 18+ (uses global fetch)
 *
 * Usage:
 *   node consumer-node.mjs
 *
 * Environment variables (or edit the defaults below):
 *   PROXY_URL       - Proxy API base URL (default: http://localhost:18080)
 *   CLIENT_ID       - Your client identifier (sk-...)
 *   ENCRYPTION_KEY  - Your encryption key (base64, 32 bytes)
 */

import * as crypto from "node:crypto";

// ─── Configuration ────────────────────────────────────────────

const PROXY_URL = process.env.PROXY_URL || "http://localhost:18080";
const CLIENT_ID = process.env.CLIENT_ID || "";
const ENCRYPTION_KEY = process.env.ENCRYPTION_KEY || "";

// ─── X-Auth Header Generation ─────────────────────────────────

/**
 * Generates the X-Auth header for authenticating with the AI Proxy.
 *
 * The encrypted payload format is:  "client_id:timestamp:nonce"
 *
 * Encryption steps:
 *   1. Base64-decode the encryption_key → 32-byte AES key
 *   2. Build plaintext payload:  client_id:timestamp:nonce
 *   3. AES-256-GCM encrypt with a random 12-byte IV
 *   4. Prepend IV to ciphertext (matching Go's Seal prepend behavior)
 *   5. Base64 URL-safe encode without padding
 *
 * @param {string} clientId - The client identifier (sk-...)
 * @param {string} encryptionKey - Base64 URL-safe encoded 32-byte key
 * @param {number} timestamp - Current Unix epoch seconds
 * @param {string} nonce - Unique per-request nonce (UUID recommended)
 * @returns {string} The X-Auth header value
 */
function generateXAuth(clientId, encryptionKey, timestamp, nonce) {
  // Decode the base64 URL-safe encryption key to 32 bytes
  const key = Buffer.from(encryptionKey, "base64url");

  if (key.length !== 32) {
    throw new Error(
      `Invalid encryption key: expected 32 bytes, got ${key.length}`
    );
  }

  // Build plaintext payload
  const payload = `${clientId}:${timestamp}:${nonce}`;

  // AES-256-GCM requires a 12-byte IV (nonce)
  const iv = crypto.randomBytes(12);

  // Encrypt with AES-256-GCM
  const cipher = crypto.createCipheriv("aes-256-gcm", key, iv);
  const encrypted = Buffer.concat([
    cipher.update(payload, "utf-8"),
    cipher.final(),
  ]);
  const tag = cipher.getAuthTag();

  // Prepend IV to match Go's gcm.Seal(nonce, ...) format: nonce || ciphertext || tag
  const combined = Buffer.concat([iv, encrypted, tag]);

  // Base64 URL-safe encode without padding
  return combined.toString("base64url");
}

// ─── Proxy Request ────────────────────────────────────────────

/**
 * Sends a chat completion request to the AI Proxy using X-Auth auth.
 *
 * @param {object} options
 * @param {string} options.model - Model name (e.g. "gpt-4")
 * @param {Array} options.messages - Chat messages
 * @param {boolean} [options.stream=false] - Enable SSE streaming
 * @returns {Promise<object>} The proxy response
 */
async function chatCompletion({ model, messages, stream = false }) {
  const timestamp = Math.floor(Date.now() / 1000);
  const nonce = crypto.randomUUID(); // UUID v4 — guaranteed unique

  const xAuth = generateXAuth(CLIENT_ID, ENCRYPTION_KEY, timestamp, nonce);

  const response = await fetch(`${PROXY_URL}/api/v1/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Client-ID": CLIENT_ID,
      "X-Auth": xAuth,
    },
    body: JSON.stringify({ model, messages, stream }),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new Error(
      `Proxy error (${response.status}): ${error?.error?.detail || response.statusText}`
    );
  }

  return response.json();
}

// ─── Streaming Request (SSE) ──────────────────────────────────

/**
 * Sends a streaming chat completion request and processes SSE chunks.
 *
 * @param {object} options
 * @param {string} options.model - Model name
 * @param {Array} options.messages - Chat messages
 * @param {function} options.onChunk - Called with each content delta
 * @returns {Promise<void>}
 */
async function chatCompletionStream({ model, messages, onChunk }) {
  const timestamp = Math.floor(Date.now() / 1000);
  const nonce = crypto.randomUUID();

  const xAuth = generateXAuth(CLIENT_ID, ENCRYPTION_KEY, timestamp, nonce);

  const response = await fetch(`${PROXY_URL}/api/v1/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Client-ID": CLIENT_ID,
      "X-Auth": xAuth,
    },
    body: JSON.stringify({ model, messages, stream: true }),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new Error(
      `Proxy error (${response.status}): ${error?.error?.detail || response.statusText}`
    );
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (const line of lines) {
      if (line.startsWith("data: ")) {
        const data = line.slice(6);
        if (data === "[DONE]") return;
        try {
          const parsed = JSON.parse(data);
          const content = parsed?.choices?.[0]?.delta?.content;
          if (content) onChunk(content);
        } catch {
          // Skip malformed SSE lines
        }
      }
    }
  }
}

// ─── Example Usage ────────────────────────────────────────────

async function main() {
  if (!CLIENT_ID || !ENCRYPTION_KEY) {
    console.error("Please set CLIENT_ID and ENCRYPTION_KEY environment variables.");
    console.error("");
    console.error("  export CLIENT_ID=sk-your-client-id");
    console.error("  export ENCRYPTION_KEY=your-base64-encoded-32-byte-key");
    console.error("  node consumer-node.mjs");
    process.exit(1);
  }

  console.log("AI Proxy Consumer SDK — Node.js Example");
  console.log(`  Proxy URL:  ${PROXY_URL}`);
  console.log(`  Client ID:  ${CLIENT_ID.slice(0, 20)}...`);
  console.log("");

  // ── Non-streaming request ────────────────────────────────
  console.log("── Non-streaming chat completion ──");

  try {
    const result = await chatCompletion({
      model: "gpt-4",
      messages: [{ role: "user", content: "Hello! Tell me a short joke." }],
    });

    console.log("  Response:");
    console.log(`    ${result?.choices?.[0]?.message?.content || "(no content)"}`);
  } catch (err) {
    console.error("  ✗ Failed:", err.message);
  }

  // ── Streaming request ────────────────────────────────────
  console.log("");
  console.log("── Streaming chat completion ──");
  console.log("  Response:", "");

  try {
    let streamedContent = "";
    await chatCompletionStream({
      model: "gpt-4",
      messages: [{ role: "user", content: "Count from 1 to 5." }],
      onChunk: (chunk) => {
        streamedContent += chunk;
        process.stdout.write(chunk);
      },
    });
    console.log("");
  } catch (err) {
    console.error("  ✗ Failed:", err.message);
  }
}

main();
