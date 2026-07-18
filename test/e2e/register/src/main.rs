use anyhow::{Context, Result, bail};
use bitwarden_core::Client;
use bitwarden_crypto::Kdf;
use serde_json::json;

#[tokio::main]
async fn main() -> Result<()> {
    let mut arguments = std::env::args().skip(1);
    let server = arguments
        .next()
        .context("server URL argument is required")?;
    let email = arguments
        .next()
        .context("email argument is required")?
        .trim()
        .to_lowercase();
    let password = arguments.next().context("password argument is required")?;
    let ca_path = arguments
        .next()
        .context("CA certificate argument is required")?;
    if arguments.next().is_some() {
        bail!("unexpected registration arguments");
    }

    let keys = Client::new(None)
        .auth()
        .make_register_keys(email.clone(), password, Kdf::default_pbkdf2())
        .context("derive account registration keys")?;
    let ca =
        reqwest::Certificate::from_pem(&std::fs::read(&ca_path).context("read CA certificate")?)
            .context("parse CA certificate")?;
    let response = reqwest::Client::builder()
        .add_root_certificate(ca)
        .build()
        .context("build registration HTTP client")?
        .post(format!(
            "{}/identity/accounts/register",
            server.trim_end_matches('/')
        ))
        .json(&json!({
            "email": email,
            "name": "bwkp e2e",
            "masterPasswordHash": keys.master_password_hash.to_string(),
            "key": keys.encrypted_user_key.to_string(),
            "keys": {
                "encryptedPrivateKey": keys.keys.private.to_string(),
                "publicKey": keys.keys.public.to_string(),
            },
            "kdf": 0,
            "kdfIterations": 600000,
        }))
        .send()
        .await
        .context("register test account")?;
    if !response.status().is_success() {
        let status = response.status();
        let body = response.text().await.unwrap_or_default();
        bail!("registration failed with {status}: {body}");
    }
    Ok(())
}
