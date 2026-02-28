import crypto from "crypto";
import * as z from "zod";

export namespace Config {
  const configSchema = z.object({
    id: z.number(),
    webhook_secret: z.string(),
    pem: z.string(),
    client_secret: z.string(),
  });

  export const load = async (env: Env) => {
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

    return configSchema.parse(JSON.parse(decrypted));
  };
}
