//! SSO Service Rust Client SDK
//!
//! 提供类型安全的 SSO 服务客户端，支持 Token 自动管理。
//!
//! # 示例
//!
//! ```rust,no_run
//! use sso_sdk::SSOClient;
//!
//! #[tokio::main]
//! async fn main() -> Result<(), sso_sdk::SSOError> {
//!     let client = SSOClient::new("http://localhost:9090");
//!
//!     // 登录
//!     client.login("user@example.com", "P@ssw0rd1").await?;
//!
//!     // 获取用户信息
//!     let info = client.user_info().await?;
//!     println!("{}", info.email);
//!
//!     // 登出
//!     client.revoke_token().await?;
//!     Ok(())
//! }
//! ```

mod client;
mod errors;
mod models;

pub use client::SSOClient;
pub use errors::{ErrorCode, SSOError};
pub use models::*;
