use std::{
    collections::HashMap,
    str::FromStr,
    sync::{Arc, Mutex},
};

use anyhow::{Context, Result, anyhow, bail};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use bitwarden_collections::collection::Collection;
use bitwarden_core::{
    Client, ClientSettings, DeviceType, OrganizationId,
    auth::login::{PasswordLoginRequest, TwoFactorProvider, TwoFactorRequest},
    client::FromClientPart,
    key_management::crypto::InitOrgCryptoRequest,
};
use bitwarden_crypto::UnsignedSharedKey;
use bitwarden_sync::{SyncClientExt, SyncRequest};
use bitwarden_vault::{
    AttachmentView, Cipher, CipherRepromptType, CipherType, Folder, VaultClientExt,
};
use chrono::Utc;
use serde::Deserialize;
use serde_json::{Value, json};
use tokio::runtime::Runtime;
use uuid::Uuid;
use zeroize::{Zeroize, Zeroizing};

const DEVICE_NAMESPACE: Uuid = Uuid::from_u128(0x0a85_5eb0_9127_4acf_b265_3999_23a9_0e2d);
// Keep this aligned with the official CLI used by the e2e fixture. Bitwarden's
// CLI sends this version in both its User-Agent and Bitwarden-Client-Version.
const BITWARDEN_CLI_VERSION: &str = "2026.6.0";

pub struct Session {
    runtime: Runtime,
    client: Client,
    source: Source,
    attachments: Mutex<HashMap<String, AttachmentRecord>>,
}

struct AttachmentRecord {
    cipher: Cipher,
    attachment: AttachmentView,
}

struct SyncCompatibilityMiddleware;

#[async_trait::async_trait]
impl reqwest_middleware::Middleware for SyncCompatibilityMiddleware {
    async fn handle(
        &self,
        request: reqwest::Request,
        extensions: &mut http::Extensions,
        next: reqwest_middleware::Next<'_>,
    ) -> Result<reqwest::Response, reqwest_middleware::Error> {
        let is_sync = request.url().path().ends_with("/sync");
        let response = next.run(request, extensions).await?;
        if !is_sync || !response.status().is_success() {
            return Ok(response);
        }
        let status = response.status();
        let version = response.version();
        let headers = response.headers().clone();
        let body = response.bytes().await?;
        let Ok(mut value) = serde_json::from_slice::<Value>(&body) else {
            return rebuild_response(status, version, headers, body.to_vec());
        };
        normalize_vaultwarden_sync(&mut value);
        let body = serde_json::to_vec(&value).map_err(reqwest_middleware::Error::middleware)?;
        rebuild_response(status, version, headers, body)
    }
}

fn normalize_vaultwarden_sync(value: &mut Value) {
    let Some(ciphers) = value.get_mut("ciphers").and_then(Value::as_array_mut) else {
        return;
    };
    for cipher in ciphers {
        let Some(data) = cipher.get_mut("data") else {
            continue;
        };
        if data.is_object() || data.is_array() {
            *data = Value::String(data.to_string());
        }
    }
}

fn rebuild_response(
    status: reqwest::StatusCode,
    version: reqwest::Version,
    headers: reqwest::header::HeaderMap,
    body: Vec<u8>,
) -> Result<reqwest::Response, reqwest_middleware::Error> {
    let mut response = http::Response::builder().status(status).version(version);
    *response
        .headers_mut()
        .expect("response builder has headers") = headers;
    response
        .body(reqwest::Body::from(body))
        .map(reqwest::Response::from)
        .map_err(reqwest_middleware::Error::middleware)
}

#[derive(Clone)]
struct Source {
    server: String,
    email: String,
}

pub enum LoginOutcome {
    Authenticated(Box<Session>),
    TwoFactor(Vec<String>),
}

#[derive(Deserialize, Zeroize)]
#[serde(rename_all = "camelCase")]
struct LoginInput {
    #[zeroize(skip)]
    endpoints: Endpoints,
    email: String,
    #[serde(deserialize_with = "decode_secret")]
    master_password: Vec<u8>,
    #[serde(default)]
    totp: String,
}

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Endpoints {
    vault_url: String,
    api_url: String,
    identity_url: String,
    #[serde(default)]
    ca_cert_pem: Vec<u8>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct AttachmentInput {
    item_id: String,
    attachment: AttachmentReference,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct AttachmentReference {
    id: String,
}

fn decode_secret<'de, D: serde::Deserializer<'de>>(deserializer: D) -> Result<Vec<u8>, D::Error> {
    let encoded = String::deserialize(deserializer)?;
    STANDARD.decode(encoded).map_err(serde::de::Error::custom)
}

pub fn login(request: &[u8]) -> Result<LoginOutcome> {
    let input: Zeroizing<LoginInput> =
        Zeroizing::new(serde_json::from_slice(request).context("decode login request")?);
    if !input.endpoints.ca_cert_pem.is_empty() {
        bail!("custom CA certificates are not yet supported by the pinned SDK adapter");
    }
    let password = std::str::from_utf8(&input.master_password)
        .context("master password is not valid UTF-8")?
        .to_owned();
    let normalized_email = input.email.trim().to_lowercase();
    let device_identifier = Uuid::new_v5(
        &DEVICE_NAMESPACE,
        format!("{}\n{}", input.endpoints.vault_url, normalized_email).as_bytes(),
    )
    .to_string();
    let settings = ClientSettings {
        identity_url: input.endpoints.identity_url.clone(),
        api_url: input.endpoints.api_url.clone(),
        user_agent: bitwarden_cli_user_agent(),
        device_type: platform_device_type(),
        device_identifier: Some(device_identifier),
        bitwarden_client_version: Some(BITWARDEN_CLI_VERSION.to_owned()),
        bitwarden_package_type: None,
    };
    let client = Client::builder()
        .with_settings(settings)
        .with_token_handler(Arc::new(
            bitwarden_auth::token_management::PasswordManagerTokenHandler::default(),
        ))
        .with_middleware(vec![Arc::new(SyncCompatibilityMiddleware)])
        .build();
    let runtime = Runtime::new().context("create Bitwarden async runtime")?;
    let response = runtime
        .block_on(client.auth().login_password(&PasswordLoginRequest {
            email: normalized_email.clone(),
            password,
            two_factor: (!input.totp.is_empty()).then(|| TwoFactorRequest {
                token: input.totp.clone(),
                provider: TwoFactorProvider::Authenticator,
                remember: false,
            }),
        }))
        .context("password login")?;

    if let Some(providers) = response.two_factor {
        let mut available = Vec::new();
        if providers.authenticator.is_some() {
            available.push("authenticator".to_owned());
        }
        if providers.email.is_some() {
            available.push("email".to_owned());
        }
        if providers.duo.is_some() {
            available.push("duo".to_owned());
        }
        if providers.organization_duo.is_some() {
            available.push("organization-duo".to_owned());
        }
        if providers.yubi_key.is_some() {
            available.push("yubikey".to_owned());
        }
        if providers.web_authn.is_some() {
            available.push("webauthn".to_owned());
        }
        return Ok(LoginOutcome::TwoFactor(available));
    }
    if !response.authenticated {
        bail!("Bitwarden rejected the login request");
    }
    Ok(LoginOutcome::Authenticated(Box::new(Session {
        runtime,
        client,
        source: Source {
            server: input.endpoints.vault_url.clone(),
            email: normalized_email,
        },
        attachments: Mutex::new(HashMap::new()),
    })))
}

pub fn sync(session: &Session) -> Result<Vec<u8>> {
    session.runtime.block_on(sync_async(session))
}

async fn sync_async(session: &Session) -> Result<Vec<u8>> {
    let response = session
        .client
        .sync()
        .sync(SyncRequest {
            exclude_subdomains: None,
        })
        .await
        .context("request full sync")?;
    initialize_organizations(&session.client, &response).await?;

    let profile = response.profile.as_deref();
    let user_id = profile
        .and_then(|value| value.id)
        .map(|value| value.to_string())
        .unwrap_or_default();
    let organization_models = response
        .organizations_new
        .as_ref()
        .or_else(|| profile.and_then(|value| value.organizations.as_ref()));
    let organizations: Vec<Value> = organization_models
        .into_iter()
        .flatten()
        .filter_map(|organization| {
            Some(json!({
                "id": organization.id?.to_string(),
                "name": organization.name.clone().unwrap_or_default(),
            }))
        })
        .collect();

    let mut folders = Vec::new();
    for model in response.folders.unwrap_or_default() {
        let encrypted = Folder::try_from(model).context("parse encrypted folder")?;
        #[allow(deprecated)]
        let view = session
            .client
            .vault()
            .folders()
            .decrypt(encrypted)
            .context("decrypt folder")?;
        folders.push(json!({
            "id": view.id.map(|value| value.to_string()).unwrap_or_default(),
            "name": view.name,
        }));
    }

    let mut collections = Vec::new();
    for model in response.collections.unwrap_or_default() {
        let encrypted = Collection::try_from(model).context("parse encrypted collection")?;
        let view = session
            .client
            .vault()
            .collections()
            .decrypt(encrypted)
            .context("decrypt collection")?;
        collections.push(json!({
            "id": view.id.map(|value| value.to_string()).unwrap_or_default(),
            "organizationId": view.organization_id.to_string(),
            "name": view.name,
        }));
    }

    let mut items = Vec::new();
    let mut attachment_records = HashMap::new();
    for model in response.ciphers.unwrap_or_default() {
        let encrypted = Cipher::try_from(model).context("parse encrypted cipher")?;
        let view = session
            .client
            .vault()
            .ciphers()
            .decrypt(encrypted.clone())
            .await
            .context("decrypt cipher")?;
        if view
            .attachment_decryption_failures
            .as_ref()
            .is_some_and(|failures| !failures.is_empty())
        {
            bail!(
                "one or more attachment descriptors failed to decrypt for cipher {:?}",
                view.id
            );
        }
        let item_id = view.id.map(|value| value.to_string()).unwrap_or_default();
        for attachment in view.attachments.clone().unwrap_or_default() {
            if let Some(attachment_id) = attachment.id.clone() {
                attachment_records.insert(
                    attachment_key(&item_id, &attachment_id),
                    AttachmentRecord {
                        cipher: encrypted.clone(),
                        attachment,
                    },
                );
            }
        }
        let mut value = serde_json::to_value(&view).context("serialize decrypted cipher")?;
        normalize_cipher(&session.client, &view, &mut value)?;
        items.push(value);
    }
    *session
        .attachments
        .lock()
        .map_err(|_| anyhow!("attachment registry lock poisoned"))? = attachment_records;

    serde_json::to_vec(&json!({
        "source": {
            "server": session.source.server,
            "email": session.source.email,
            "userId": user_id,
            "syncedAt": Utc::now(),
        },
        "folders": folders,
        "collections": collections,
        "organizations": organizations,
        "items": items,
    }))
    .context("encode decrypted vault snapshot")
}

async fn initialize_organizations(
    client: &Client,
    response: &bitwarden_api_api::models::SyncResponseModel,
) -> Result<()> {
    let profile = response.profile.as_deref();
    let models = response
        .organizations_new
        .as_ref()
        .or_else(|| profile.and_then(|value| value.organizations.as_ref()));
    let mut organization_keys = HashMap::new();
    for organization in models.into_iter().flatten() {
        if let (Some(id), Some(key)) = (organization.id, organization.key.as_deref()) {
            organization_keys.insert(
                OrganizationId::new(id),
                UnsignedSharedKey::from_str(key).context("parse organization shared key")?,
            );
        }
    }
    client
        .crypto()
        .initialize_org_crypto(InitOrgCryptoRequest { organization_keys })
        .await
        .context("initialize organization cryptography")
}

fn normalize_cipher(
    client: &Client,
    view: &bitwarden_vault::CipherView,
    value: &mut Value,
) -> Result<()> {
    let object = value
        .as_object_mut()
        .context("cipher did not serialize as an object")?;
    object.insert(
        "type".to_owned(),
        Value::String(cipher_type(view.r#type).to_owned()),
    );
    object.insert(
        "reprompt".to_owned(),
        Value::Bool(view.reprompt == CipherRepromptType::Password),
    );
    if let Some(login) = object.get_mut("login").and_then(Value::as_object_mut)
        && view
            .login
            .as_ref()
            .and_then(|value| value.fido2_credentials.as_ref())
            .is_some()
    {
        let credentials = view
            .get_fido2_credentials(&mut client.internal.get_key_store().context())
            .context("decrypt passkey credentials")?;
        login.insert(
            "fido2Credentials".to_owned(),
            serde_json::to_value(credentials)?,
        );
    }
    if let Some(attachments) = object.get_mut("attachments").and_then(Value::as_array_mut) {
        for attachment in attachments {
            if let Some(object) = attachment.as_object_mut()
                && let Some(size) = object.get("size").and_then(Value::as_str)
            {
                object.insert(
                    "size".to_owned(),
                    Value::Number(size.parse::<i64>().unwrap_or_default().into()),
                );
            }
        }
    }
    let data = match view.r#type {
        CipherType::BankAccount => object.get("bankAccount").cloned(),
        CipherType::DriversLicense => object.get("driversLicense").cloned(),
        CipherType::Passport => object.get("passport").cloned(),
        _ => None,
    };
    if let Some(data) = data {
        object.insert("data".to_owned(), data);
    }
    Ok(())
}

fn cipher_type(value: CipherType) -> &'static str {
    match value {
        CipherType::Login => "login",
        CipherType::SecureNote => "secureNote",
        CipherType::Card => "card",
        CipherType::Identity => "identity",
        CipherType::SshKey => "sshKey",
        CipherType::BankAccount => "bankAccount",
        CipherType::DriversLicense => "driversLicense",
        CipherType::Passport => "passport",
    }
}

pub fn download_attachment(session: &Session, request: &[u8]) -> Result<Vec<u8>> {
    let input: AttachmentInput =
        serde_json::from_slice(request).context("decode attachment request")?;
    session
        .runtime
        .block_on(download_attachment_async(session, input))
}

async fn download_attachment_async(session: &Session, input: AttachmentInput) -> Result<Vec<u8>> {
    let key = attachment_key(&input.item_id, &input.attachment.id);
    let (cipher, attachment) = {
        let records = session
            .attachments
            .lock()
            .map_err(|_| anyhow!("attachment registry lock poisoned"))?;
        let record = records.get(&key).with_context(|| {
            format!(
                "attachment {} is not in the synced vault",
                input.attachment.id
            )
        })?;
        (record.cipher.clone(), record.attachment.clone())
    };
    let cipher_id = Uuid::parse_str(&input.item_id).context("parse cipher ID")?;
    let configurations: std::sync::Arc<bitwarden_core::client::ApiConfigurations> =
        session.client.get_part();
    let descriptor = configurations
        .api_client
        .ciphers_api()
        .get_attachment_data(cipher_id, &input.attachment.id)
        .await
        .context("request attachment download URL")?;
    let url = descriptor
        .url
        .or(attachment.url.clone())
        .context("server returned no attachment URL")?;
    let http_client = session.client.internal.get_http_client();
    let mut response = http_client
        .execute(attachment_download_request(http_client, url)?)
        .await
        .context("download encrypted attachment")?
        .error_for_status()
        .context("attachment download status")?;
    let mut encrypted = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .context("read encrypted attachment")?
    {
        encrypted.extend_from_slice(&chunk);
    }
    let decrypted = session
        .client
        .vault()
        .attachments()
        .decrypt_buffer(cipher, attachment, &encrypted)
        .context("decrypt attachment")?;
    encrypted.zeroize();
    Ok(decrypted)
}

fn attachment_download_request(client: &reqwest::Client, url: String) -> Result<reqwest::Request> {
    client
        .get(url)
        .header(reqwest::header::USER_AGENT, bitwarden_cli_user_agent())
        .header("Bitwarden-Client-Name", "cli")
        .header("Bitwarden-Client-Version", BITWARDEN_CLI_VERSION)
        .header("Device-Type", (platform_device_type() as u8).to_string())
        .build()
        .context("build attachment download request")
}

fn attachment_key(item_id: &str, attachment_id: &str) -> String {
    format!("{item_id}\0{attachment_id}")
}

fn platform_device_type() -> DeviceType {
    if cfg!(target_os = "windows") {
        DeviceType::WindowsCLI
    } else if cfg!(target_os = "macos") {
        DeviceType::MacOsCLI
    } else {
        DeviceType::LinuxCLI
    }
}

fn bitwarden_cli_user_agent() -> String {
    let platform = if cfg!(target_os = "windows") {
        "WINDOWS"
    } else if cfg!(target_os = "macos") {
        "MACOS"
    } else {
        "LINUX"
    };
    format!("Bitwarden_CLI/{BITWARDEN_CLI_VERSION} ({platform})")
}

#[cfg(test)]
mod tests {
    use super::{
        BITWARDEN_CLI_VERSION, attachment_download_request, bitwarden_cli_user_agent,
        normalize_vaultwarden_sync, platform_device_type,
    };
    use serde_json::{Value, json};

    #[test]
    fn normalizes_legacy_cipher_data_objects_and_arrays() {
        let mut response = json!({
            "ciphers": [
                {"data": {"username": "alice", "password": "secret"}},
                {"data": ["one", "two"]}
            ]
        });

        normalize_vaultwarden_sync(&mut response);

        assert_eq!(
            response["ciphers"][0]["data"],
            Value::String(r#"{"password":"secret","username":"alice"}"#.to_owned())
        );
        assert_eq!(
            response["ciphers"][1]["data"],
            Value::String(r#"["one","two"]"#.to_owned())
        );
    }

    #[test]
    fn preserves_sdk_native_and_unrelated_sync_data() {
        let mut response = json!({
            "ciphers": [
                {"data": "already encrypted"},
                {"name": "without legacy data"}
            ],
            "profile": {"name": "Example"}
        });
        let expected = response.clone();

        normalize_vaultwarden_sync(&mut response);

        assert_eq!(response, expected);
    }

    #[test]
    fn accepts_sync_responses_without_cipher_arrays() {
        for mut response in [json!({}), json!({"ciphers": null}), json!({"ciphers": {}})] {
            let expected = response.clone();
            normalize_vaultwarden_sync(&mut response);
            assert_eq!(response, expected);
        }
    }

    #[test]
    fn uses_official_bitwarden_cli_user_agent_shape() {
        let expected_platform = if cfg!(target_os = "windows") {
            "WINDOWS"
        } else if cfg!(target_os = "macos") {
            "MACOS"
        } else {
            "LINUX"
        };

        assert_eq!(
            bitwarden_cli_user_agent(),
            format!("Bitwarden_CLI/{BITWARDEN_CLI_VERSION} ({expected_platform})")
        );
    }

    #[test]
    fn attachment_download_uses_official_client_headers_without_credentials() {
        let client = reqwest::Client::new();
        let request = attachment_download_request(
            &client,
            "https://vault.example/attachments/cipher/file?token=signed".to_owned(),
        )
        .expect("attachment request");

        assert_eq!(
            request.headers()[reqwest::header::USER_AGENT],
            bitwarden_cli_user_agent()
        );
        assert_eq!(request.headers()["Bitwarden-Client-Name"], "cli");
        assert_eq!(
            request.headers()["Bitwarden-Client-Version"],
            BITWARDEN_CLI_VERSION
        );
        assert_eq!(
            request.headers()["Device-Type"],
            (platform_device_type() as u8).to_string()
        );
        assert!(
            !request
                .headers()
                .contains_key(reqwest::header::AUTHORIZATION)
        );
        assert!(!request.headers().contains_key("Bitwarden-Package-Type"));
    }
}
