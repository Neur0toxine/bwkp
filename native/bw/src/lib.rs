use std::{
    collections::HashMap,
    str::FromStr,
    sync::{Arc, Mutex},
};

use anyhow::{Context, Result, anyhow, bail};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use bitwarden_api_api::models::{
    AttachmentRequestModel, CipherRequestModel, CipherResponseModel, FileUploadType,
};
use bitwarden_api_base::AuthRequired;
use bitwarden_collections::collection::Collection;
use bitwarden_core::{
    Client, ClientSettings, DeviceType, OrganizationId, UserId,
    auth::login::{PasswordLoginRequest, TwoFactorProvider, TwoFactorRequest},
    client::FromClientPart,
    key_management::crypto::InitOrgCryptoRequest,
};
use bitwarden_crypto::{IdentifyKey, UnsignedSharedKey};
use bitwarden_sync::{SyncClientExt, SyncRequest};
use bitwarden_vault::{
    AttachmentView, BankAccountView, CardView, Cipher, CipherId, CipherRepromptType, CipherType,
    CipherView, CipherViewType, DriversLicenseView, Fido2CredentialFullView, FieldView, Folder,
    FolderAddEditRequest, FolderId, IdentityView, LoginUriView, LoginView, PassportView,
    SecureNoteType, SecureNoteView, SshKeyView, VaultClientExt,
};
use chrono::{DateTime, Utc};
use reqwest::multipart::{Form, Part};
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
    ciphers: Mutex<HashMap<String, Cipher>>,
    views: Mutex<HashMap<String, CipherView>>,
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
        let path = request.url().path();
        let normalize_response = path.ends_with("/sync") || path.contains("/ciphers");
        let response = next.run(request, extensions).await?;
        if !normalize_response || !response.status().is_success() {
            return Ok(response);
        }
        let status = response.status();
        let version = response.version();
        let headers = response.headers().clone();
        let body = response.bytes().await?;
        let Ok(mut value) = serde_json::from_slice::<Value>(&body) else {
            return rebuild_response(status, version, headers, body.to_vec());
        };
        normalize_vaultwarden_response(&mut value);
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

fn normalize_vaultwarden_response(value: &mut Value) {
    normalize_nested_cipher_data(value);
    normalize_vaultwarden_sync(value);
}

fn normalize_nested_cipher_data(value: &mut Value) {
    match value {
        Value::Object(object) => {
            for (name, child) in object {
                if name.eq_ignore_ascii_case("data") && (child.is_object() || child.is_array()) {
                    *child = Value::String(child.to_string());
                } else {
                    normalize_nested_cipher_data(child);
                }
            }
        }
        Value::Array(values) => {
            for child in values {
                normalize_nested_cipher_data(child);
            }
        }
        _ => {}
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
        ciphers: Mutex::new(HashMap::new()),
        views: Mutex::new(HashMap::new()),
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
    if let Some(id) = profile.and_then(|value| value.id) {
        session
            .client
            .internal
            .init_user_id(UserId::new(id))
            .await
            .context("initialize synced user ID")?;
    }
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
    let mut cipher_records = HashMap::new();
    let mut view_records = HashMap::new();
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
        cipher_records.insert(item_id.clone(), encrypted);
        view_records.insert(item_id, view);
    }
    *session
        .attachments
        .lock()
        .map_err(|_| anyhow!("attachment registry lock poisoned"))? = attachment_records;
    *session
        .ciphers
        .lock()
        .map_err(|_| anyhow!("cipher registry lock poisoned"))? = cipher_records;
    *session
        .views
        .lock()
        .map_err(|_| anyhow!("cipher view registry lock poisoned"))? = view_records;

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

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ImportItem {
    name: String,
    #[serde(default)]
    notes: Option<String>,
    #[serde(default)]
    favorite: bool,
    #[serde(default)]
    reprompt: bool,
    r#type: String,
    #[serde(default)]
    login: Option<ImportLogin>,
    #[serde(default)]
    card: Option<Value>,
    #[serde(default)]
    identity: Option<Value>,
    #[serde(default)]
    ssh_key: Option<Value>,
    #[serde(default)]
    data: Option<Value>,
    #[serde(default)]
    fields: Vec<FieldView>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ImportLogin {
    #[serde(default)]
    username: Option<String>,
    #[serde(default)]
    password: Option<String>,
    #[serde(default)]
    totp: Option<String>,
    #[serde(default)]
    uris: Vec<ImportUri>,
    #[serde(default)]
    fido2_credentials: Vec<ImportFido2Credential>,
}

#[derive(Deserialize)]
struct ImportUri {
    uri: String,
    #[serde(default)]
    r#match: Option<i32>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ImportFido2Credential {
    credential_id: String,
    key_value: String,
    rp_id: String,
    #[serde(default)]
    rp_name: Option<String>,
    #[serde(default)]
    user_handle: Option<String>,
    #[serde(default)]
    user_name: Option<String>,
    #[serde(default)]
    user_display_name: Option<String>,
    #[serde(default)]
    counter: Option<String>,
    #[serde(default)]
    discoverable: Option<String>,
    #[serde(default)]
    creation_date: Option<DateTime<Utc>>,
}

#[derive(Deserialize)]
#[serde(
    tag = "action",
    rename_all = "camelCase",
    rename_all_fields = "camelCase"
)]
enum MutationInput {
    CreateFolder {
        name: String,
    },
    CreateItem {
        item: ImportItem,
        #[serde(default)]
        folder_id: Option<String>,
    },
    UpdateItem {
        id: String,
        item: ImportItem,
        #[serde(default)]
        folder_id: Option<String>,
    },
    TrashItem {
        id: String,
    },
    RestoreItem {
        id: String,
    },
    ArchiveItem {
        id: String,
    },
    UnarchiveItem {
        id: String,
    },
    DeleteAttachment {
        item_id: String,
        attachment_id: String,
    },
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct UploadInput {
    item_id: String,
    file_name: String,
}

pub fn mutate(session: &Session, request: &[u8]) -> Result<Vec<u8>> {
    let input: MutationInput =
        serde_json::from_slice(request).context("decode mutation request")?;
    session.runtime.block_on(mutate_async(session, input))
}

async fn mutate_async(session: &Session, input: MutationInput) -> Result<Vec<u8>> {
    match input {
        MutationInput::CreateFolder { name } => {
            let folder = session
                .client
                .vault()
                .folders()
                .create(FolderAddEditRequest { name })
                .await
                .context("create folder")?;
            serde_json::to_vec(&json!({
                "id": folder.id.map(|value| value.to_string()).unwrap_or_default(),
                "name": folder.name,
            }))
            .context("encode created folder")
        }
        MutationInput::CreateItem { item, folder_id } => {
            let mut view = create_import_item(session, item, folder_id.clone()).await?;
            if folder_id.is_some() {
                move_import_item(session, &mut view, folder_id).await?;
            }
            cache_view(session, view.clone())?;
            encode_item_result(&view)
        }
        MutationInput::UpdateItem {
            id,
            item,
            folder_id,
        } => {
            let existing_view = session
                .views
                .lock()
                .map_err(|_| anyhow!("cipher view registry lock poisoned"))?
                .get(&id)
                .cloned()
                .context("item is not in the synced vault")?;
            let existing_cipher = session
                .ciphers
                .lock()
                .map_err(|_| anyhow!("cipher registry lock poisoned"))?
                .get(&id)
                .cloned()
                .context("encrypted item is not in the synced vault")?;
            let mut view = update_import_item(
                session,
                id.parse()?,
                item,
                folder_id.clone(),
                existing_view,
                existing_cipher,
            )
            .await?;
            move_import_item(session, &mut view, folder_id).await?;
            cache_view(session, view.clone())?;
            encode_item_result(&view)
        }
        MutationInput::TrashItem { id } => {
            session
                .client
                .vault()
                .ciphers()
                .soft_delete(id.parse()?)
                .await
                .context("move item to trash")?;
            Ok(b"{}".to_vec())
        }
        MutationInput::RestoreItem { id } => {
            let view = session
                .client
                .vault()
                .ciphers()
                .restore(id.parse()?)
                .await
                .context("restore item")?;
            cache_view(session, view)?;
            Ok(b"{}".to_vec())
        }
        MutationInput::ArchiveItem { id } => {
            let configurations: Arc<bitwarden_core::client::ApiConfigurations> =
                session.client.get_part();
            configurations
                .api_client
                .ciphers_api()
                .put_archive(id.parse()?)
                .await
                .context("archive item")?;
            Ok(b"{}".to_vec())
        }
        MutationInput::UnarchiveItem { id } => {
            let configurations: Arc<bitwarden_core::client::ApiConfigurations> =
                session.client.get_part();
            configurations
                .api_client
                .ciphers_api()
                .put_unarchive(id.parse()?)
                .await
                .context("unarchive item")?;
            if let Some(cipher) = session
                .ciphers
                .lock()
                .map_err(|_| anyhow!("cipher registry lock poisoned"))?
                .get_mut(&id)
            {
                cipher.archived_date = None;
            }
            if let Some(view) = session
                .views
                .lock()
                .map_err(|_| anyhow!("cipher view registry lock poisoned"))?
                .get_mut(&id)
            {
                view.archived_date = None;
            }
            Ok(b"{}".to_vec())
        }
        MutationInput::DeleteAttachment {
            item_id,
            attachment_id,
        } => {
            session
                .client
                .vault()
                .ciphers()
                .delete_attachment(item_id.parse()?, attachment_id)
                .await
                .context("delete attachment")?;
            Ok(b"{}".to_vec())
        }
    }
}

async fn move_import_item(
    session: &Session,
    view: &mut CipherView,
    folder_id: Option<String>,
) -> Result<()> {
    let item_id = view.id.context("mutated item has no ID")?;
    let folder_id = folder_id.map(|value| value.parse()).transpose()?;
    session
        .client
        .vault()
        .ciphers()
        .move_many(vec![item_id], folder_id)
        .await
        .context("move imported item to folder")?;
    view.folder_id = folder_id;
    Ok(())
}

async fn create_import_item(
    session: &Session,
    item: ImportItem,
    folder_id: Option<String>,
) -> Result<CipherView> {
    let now = Utc::now();
    let (view_type, credentials) = import_view_type(&item)?;
    let type_fields = split_view_type(view_type);
    let mut view = CipherView {
        id: None,
        organization_id: None,
        folder_id: folder_id.map(|value| value.parse()).transpose()?,
        collection_ids: vec![],
        key: None,
        name: item.name,
        notes: item.notes,
        r#type: type_fields.0,
        login: type_fields.1,
        identity: type_fields.2,
        card: type_fields.3,
        secure_note: type_fields.4,
        ssh_key: type_fields.5,
        bank_account: type_fields.6,
        drivers_license: type_fields.7,
        passport: type_fields.8,
        favorite: item.favorite,
        reprompt: reprompt(item.reprompt),
        organization_use_totp: false,
        edit: true,
        permissions: None,
        view_password: true,
        local_data: None,
        attachments: None,
        attachment_decryption_failures: None,
        fields: Some(item.fields),
        password_history: None,
        creation_date: now,
        deleted_date: None,
        revision_date: now,
        archived_date: None,
    };
    let key_store = session.client.internal.get_key_store();
    let key = view.key_identifier();
    view.generate_cipher_key(&mut key_store.context(), key)?;
    if !credentials.is_empty() {
        view.set_new_fido2_credentials(&mut key_store.context(), credentials)?;
    }
    let encrypted: Cipher = key_store.encrypt(view)?;
    save_cipher(session, encrypted, None).await
}

async fn update_import_item(
    session: &Session,
    id: CipherId,
    item: ImportItem,
    folder_id: Option<String>,
    existing_view: CipherView,
    existing_cipher: Cipher,
) -> Result<CipherView> {
    let (view_type, credentials) = import_view_type(&item)?;
    let type_fields = split_view_type(view_type);
    let mut view = CipherView {
        id: Some(id),
        organization_id: None,
        folder_id: folder_id.map(|value| value.parse()).transpose()?,
        collection_ids: vec![],
        key: existing_view.key,
        name: item.name,
        notes: item.notes,
        r#type: type_fields.0,
        login: type_fields.1,
        identity: type_fields.2,
        card: type_fields.3,
        secure_note: type_fields.4,
        ssh_key: type_fields.5,
        bank_account: type_fields.6,
        drivers_license: type_fields.7,
        passport: type_fields.8,
        favorite: item.favorite,
        reprompt: reprompt(item.reprompt),
        organization_use_totp: false,
        edit: true,
        permissions: existing_view.permissions,
        view_password: true,
        local_data: existing_view.local_data,
        attachments: existing_view.attachments,
        attachment_decryption_failures: None,
        fields: Some(item.fields),
        password_history: existing_view.password_history,
        creation_date: existing_view.creation_date,
        deleted_date: None,
        revision_date: existing_view.revision_date,
        archived_date: existing_view.archived_date,
    };
    let key_store = session.client.internal.get_key_store();
    if !credentials.is_empty() {
        view.set_new_fido2_credentials(&mut key_store.context(), credentials)?;
    }
    let encrypted: Cipher = key_store.encrypt(view)?;
    save_cipher(session, encrypted, Some(existing_cipher)).await
}

async fn save_cipher(
    session: &Session,
    mut encrypted: Cipher,
    previous: Option<Cipher>,
) -> Result<CipherView> {
    let user_id = session
        .client
        .internal
        .get_user_id()
        .context("authenticated user ID is missing")?;
    let mut request: CipherRequestModel = encrypted.clone().try_into()?;
    request.encrypted_for = Some(user_id.into());
    let configurations: Arc<bitwarden_core::client::ApiConfigurations> = session.client.get_part();
    let response = if let Some(previous) = previous {
        let id = encrypted.id.context("updated item has no ID")?;
        let response = configurations
            .api_client
            .ciphers_api()
            .put(id.into(), Some(request))
            .await
            .context("update encrypted item")?;
        encrypted.attachments = previous.attachments;
        response
    } else {
        configurations
            .api_client
            .ciphers_api()
            .post(Some(request))
            .await
            .context("create encrypted item")?
    };
    apply_cipher_response(&mut encrypted, response)?;
    session
        .client
        .internal
        .get_key_store()
        .decrypt(&encrypted)
        .context("decrypt mutated item")
}

fn apply_cipher_response(cipher: &mut Cipher, response: CipherResponseModel) -> Result<()> {
    if let Some(id) = response.id {
        cipher.id = Some(CipherId::new(id));
    }
    if let Some(folder_id) = response.folder_id {
        cipher.folder_id = Some(FolderId::new(folder_id));
    }
    if let Some(value) = response.creation_date {
        cipher.creation_date = value.parse()?;
    }
    if let Some(value) = response.revision_date {
        cipher.revision_date = value.parse()?;
    }
    cipher.deleted_date = response
        .deleted_date
        .map(|value| value.parse())
        .transpose()?;
    cipher.archived_date = response
        .archived_date
        .map(|value| value.parse())
        .transpose()?;
    Ok(())
}

#[allow(clippy::type_complexity)]
fn split_view_type(
    value: CipherViewType,
) -> (
    CipherType,
    Option<LoginView>,
    Option<IdentityView>,
    Option<CardView>,
    Option<SecureNoteView>,
    Option<SshKeyView>,
    Option<BankAccountView>,
    Option<DriversLicenseView>,
    Option<PassportView>,
) {
    match value {
        CipherViewType::Login(value) => (
            CipherType::Login,
            Some(value),
            None,
            None,
            None,
            None,
            None,
            None,
            None,
        ),
        CipherViewType::Identity(value) => (
            CipherType::Identity,
            None,
            Some(value),
            None,
            None,
            None,
            None,
            None,
            None,
        ),
        CipherViewType::Card(value) => (
            CipherType::Card,
            None,
            None,
            Some(value),
            None,
            None,
            None,
            None,
            None,
        ),
        CipherViewType::SecureNote(value) => (
            CipherType::SecureNote,
            None,
            None,
            None,
            Some(value),
            None,
            None,
            None,
            None,
        ),
        CipherViewType::SshKey(value) => (
            CipherType::SshKey,
            None,
            None,
            None,
            None,
            Some(value),
            None,
            None,
            None,
        ),
        CipherViewType::BankAccount(value) => (
            CipherType::BankAccount,
            None,
            None,
            None,
            None,
            None,
            Some(value),
            None,
            None,
        ),
        CipherViewType::DriversLicense(value) => (
            CipherType::DriversLicense,
            None,
            None,
            None,
            None,
            None,
            None,
            Some(value),
            None,
        ),
        CipherViewType::Passport(value) => (
            CipherType::Passport,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            Some(value),
        ),
    }
}

fn import_view_type(item: &ImportItem) -> Result<(CipherViewType, Vec<Fido2CredentialFullView>)> {
    let (view, credentials) = match item.r#type.as_str() {
        "login" => {
            let login = item.login.as_ref().context("login payload is missing")?;
            let uris = login
                .uris
                .iter()
                .map(|uri| {
                    serde_json::from_value::<LoginUriView>(json!({
                        "uri": uri.uri,
                        "match": uri.r#match,
                        "uriChecksum": null,
                    }))
                    .context("decode login URI")
                })
                .collect::<Result<Vec<_>>>()?;
            let credentials = login
                .fido2_credentials
                .iter()
                .map(|credential| Fido2CredentialFullView {
                    credential_id: credential.credential_id.clone(),
                    key_type: "public-key".to_owned(),
                    key_algorithm: "ECDSA".to_owned(),
                    key_curve: "P-256".to_owned(),
                    key_value: credential.key_value.clone(),
                    rp_id: credential.rp_id.clone(),
                    user_handle: credential.user_handle.clone(),
                    user_name: credential.user_name.clone(),
                    counter: credential.counter.clone().unwrap_or_else(|| "0".to_owned()),
                    rp_name: credential.rp_name.clone(),
                    user_display_name: credential.user_display_name.clone(),
                    discoverable: credential
                        .discoverable
                        .clone()
                        .unwrap_or_else(|| "false".to_owned()),
                    creation_date: credential.creation_date.unwrap_or_else(Utc::now),
                })
                .collect();
            (
                CipherViewType::Login(LoginView {
                    username: login.username.clone(),
                    password: login.password.clone(),
                    password_revision_date: None,
                    uris: (!uris.is_empty()).then_some(uris),
                    totp: login.totp.clone(),
                    autofill_on_page_load: None,
                    fido2_credentials: None,
                }),
                credentials,
            )
        }
        "card" => (
            CipherViewType::Card(serde_json::from_value::<CardView>(
                item.card.clone().unwrap_or_else(|| json!({})),
            )?),
            vec![],
        ),
        "identity" => (
            CipherViewType::Identity(serde_json::from_value::<IdentityView>(
                item.identity.clone().unwrap_or_else(|| json!({})),
            )?),
            vec![],
        ),
        "sshKey" => (
            CipherViewType::SshKey(serde_json::from_value::<SshKeyView>(
                item.ssh_key.clone().context("SSH key payload is missing")?,
            )?),
            vec![],
        ),
        "bankAccount" => (
            CipherViewType::BankAccount(serde_json::from_value(
                item.data.clone().unwrap_or_else(|| json!({})),
            )?),
            vec![],
        ),
        "driversLicense" => (
            CipherViewType::DriversLicense(serde_json::from_value(
                item.data.clone().unwrap_or_else(|| json!({})),
            )?),
            vec![],
        ),
        "passport" => (
            CipherViewType::Passport(serde_json::from_value(
                item.data.clone().unwrap_or_else(|| json!({})),
            )?),
            vec![],
        ),
        "secureNote" => (
            CipherViewType::SecureNote(SecureNoteView {
                r#type: SecureNoteType::Generic,
            }),
            vec![],
        ),
        value => bail!("unsupported Bitwarden item type {value:?}"),
    };
    Ok((view, credentials))
}

fn reprompt(value: bool) -> CipherRepromptType {
    if value {
        CipherRepromptType::Password
    } else {
        CipherRepromptType::None
    }
}

fn cache_view(session: &Session, view: CipherView) -> Result<()> {
    let id = view.id.context("mutated item has no ID")?.to_string();
    let encrypted: Cipher = session
        .client
        .internal
        .get_key_store()
        .encrypt(view.clone())?;
    session
        .ciphers
        .lock()
        .map_err(|_| anyhow!("cipher registry lock poisoned"))?
        .insert(id.clone(), encrypted);
    session
        .views
        .lock()
        .map_err(|_| anyhow!("cipher view registry lock poisoned"))?
        .insert(id, view);
    Ok(())
}

fn encode_item_result(view: &CipherView) -> Result<Vec<u8>> {
    serde_json::to_vec(&json!({
        "id": view.id.map(|value| value.to_string()).unwrap_or_default(),
        "attachments": view.attachments,
    }))
    .context("encode mutated item")
}

pub fn upload_attachment(session: &Session, request: &[u8], content: &[u8]) -> Result<Vec<u8>> {
    let input: UploadInput = serde_json::from_slice(request).context("decode upload request")?;
    session
        .runtime
        .block_on(upload_attachment_async(session, input, content))
}

async fn upload_attachment_async(
    session: &Session,
    input: UploadInput,
    content: &[u8],
) -> Result<Vec<u8>> {
    let cipher = session
        .ciphers
        .lock()
        .map_err(|_| anyhow!("cipher registry lock poisoned"))?
        .get(&input.item_id)
        .cloned()
        .context("item is not in the synced vault")?;
    let encrypted = session.client.vault().attachments().encrypt_buffer(
        cipher,
        AttachmentView {
            id: None,
            url: None,
            size: None,
            size_name: None,
            file_name: Some(input.file_name),
            key: None,
        },
        content,
    )?;
    let file_name = encrypted
        .attachment
        .file_name
        .as_ref()
        .context("encrypted attachment has no filename")?
        .to_string();
    let configurations: Arc<bitwarden_core::client::ApiConfigurations> = session.client.get_part();
    let item_id: Uuid = input.item_id.parse()?;
    let response = configurations
        .api_client
        .ciphers_api()
        .post_attachment(
            item_id,
            Some(AttachmentRequestModel {
                key: encrypted.attachment.key.map(|value| value.to_string()),
                file_name: Some(file_name.clone()),
                file_size: Some(i64::try_from(encrypted.contents.len())?),
                admin_request: None,
                last_known_revision_date: session.views.lock().ok().and_then(|views| {
                    views
                        .get(&input.item_id)
                        .map(|view| view.revision_date.to_rfc3339())
                }),
            }),
        )
        .await
        .context("create attachment descriptor")?;
    let revision_date = response
        .cipher_response
        .as_ref()
        .and_then(|value| value.revision_date.as_deref())
        .or_else(|| {
            response
                .cipher_mini_response
                .as_ref()
                .and_then(|value| value.revision_date.as_deref())
        })
        .map(str::parse::<DateTime<Utc>>)
        .transpose()?;
    if let Some(revision_date) = revision_date {
        if let Some(cipher) = session
            .ciphers
            .lock()
            .map_err(|_| anyhow!("cipher registry lock poisoned"))?
            .get_mut(&input.item_id)
        {
            cipher.revision_date = revision_date;
        }
        if let Some(view) = session
            .views
            .lock()
            .map_err(|_| anyhow!("cipher view registry lock poisoned"))?
            .get_mut(&input.item_id)
        {
            view.revision_date = revision_date;
        }
    }
    let attachment_id = response
        .attachment_id
        .context("server returned no attachment ID")?;
    let upload_result = match response.file_upload_type.unwrap_or_default() {
        FileUploadType::Direct => {
            let form = Form::new().part(
                "data",
                Part::bytes(encrypted.contents)
                    .file_name(file_name)
                    .mime_str("application/octet-stream")?,
            );
            configurations
                .api_config
                .client
                .post(format!(
                    "{}/ciphers/{}/attachment/{}",
                    configurations.api_config.base_path, item_id, attachment_id
                ))
                .with_extension(AuthRequired::Bearer)
                .header(reqwest::header::USER_AGENT, bitwarden_cli_user_agent())
                .header("Bitwarden-Client-Name", "cli")
                .header("Bitwarden-Client-Version", BITWARDEN_CLI_VERSION)
                .header("Device-Type", (platform_device_type() as u8).to_string())
                .multipart(form)
                .send()
                .await?
                .error_for_status()
                .map(|_| ())
                .context("upload attachment to Bitwarden")
        }
        FileUploadType::Azure => {
            let url = response
                .url
                .context("server returned no attachment upload URL")?;
            let parsed = reqwest::Url::parse(&url)?;
            let version = parsed
                .query_pairs()
                .find_map(|(name, value)| (name == "sv").then(|| value.into_owned()))
                .unwrap_or_else(|| "2021-08-06".to_owned());
            session
                .client
                .internal
                .get_http_client()
                .put(url)
                .header(reqwest::header::USER_AGENT, bitwarden_cli_user_agent())
                .header("Bitwarden-Client-Name", "cli")
                .header("Bitwarden-Client-Version", BITWARDEN_CLI_VERSION)
                .header("Device-Type", (platform_device_type() as u8).to_string())
                .header("x-ms-date", Utc::now().to_rfc2822())
                .header("x-ms-version", version)
                .header("x-ms-blob-type", "BlockBlob")
                .body(encrypted.contents)
                .send()
                .await?
                .error_for_status()
                .map(|_| ())
                .context("upload attachment to Azure")
        }
        FileUploadType::__Unknown(value) => bail!("unsupported attachment upload type {value}"),
    };
    if let Err(error) = upload_result {
        let rollback = session
            .client
            .vault()
            .ciphers()
            .delete_attachment(CipherId::new(item_id), attachment_id.clone())
            .await;
        return Err(error.context(format!("attachment rollback result: {rollback:?}")));
    }
    serde_json::to_vec(&json!({"id": attachment_id})).context("encode uploaded attachment")
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
        BITWARDEN_CLI_VERSION, MutationInput, attachment_download_request,
        bitwarden_cli_user_agent, normalize_vaultwarden_response, normalize_vaultwarden_sync,
        platform_device_type,
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
    fn normalizes_legacy_cipher_mutation_responses() {
        let mut response = json!({
            "data": {"username": "alice"},
            "cipherResponse": {"data": ["one"]}
        });

        normalize_vaultwarden_response(&mut response);

        assert_eq!(
            response["data"],
            Value::String(r#"{"username":"alice"}"#.to_owned())
        );
        assert_eq!(
            response["cipherResponse"]["data"],
            Value::String(r#"["one"]"#.to_owned())
        );
    }

    #[test]
    fn mutation_fields_use_camel_case() {
        let mutation: MutationInput = serde_json::from_value(json!({
            "action": "deleteAttachment",
            "itemId": "item",
            "attachmentId": "attachment"
        }))
        .unwrap();
        assert!(matches!(
            mutation,
            MutationInput::DeleteAttachment { item_id, attachment_id }
                if item_id == "item" && attachment_id == "attachment"
        ));
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
