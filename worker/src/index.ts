import crypto from "crypto";

const loadConfig = async (env: Env) => {
  const [key, encrypted] = await Promise.all([
    env.APP_KEY.get().then((key) => Buffer.from(key, "hex")),
    env.BUCKET.get("app_config.json").then((resp) => resp?.text()),
  ]);

  if (!encrypted) {
    throw new Error("app_config.json not found");
  }

  const [nonce, ciphertext, tag] = encrypted.split(":").map((part) => Buffer.from(part, "hex"));

  const decipher = crypto.createDecipheriv("aes-256-gcm", key, nonce);
  decipher.setAuthTag(tag);

  const decrypted = decipher.update(ciphertext, undefined, "utf8") + decipher.final("utf8");

  return decrypted;
};

export default {
  async fetch(request, env, ctx): Promise<Response> {
    const now = performance.now();
    const config = await loadConfig(env);
    const end = performance.now();

    console.log(end - now);

    return new Response(config);
  },
} satisfies ExportedHandler<Env>;
