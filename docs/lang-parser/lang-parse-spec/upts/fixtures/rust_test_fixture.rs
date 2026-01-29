//! Rust Parser Test Fixture
//! Tests all symbol extraction capabilities following canonical patterns
//! Line numbers are predictable for automated testing

// === Pattern 1: Constants ===
// Line 7-9
pub const MAX_RETRIES: u32 = 3;
pub const DEFAULT_TIMEOUT: u64 = 30;
const INTERNAL_BUFFER_SIZE: usize = 1024;

// === Pattern 7: Type Aliases ===
// Line 12-13
pub type UserId = String;
pub type ItemList = Vec<Item>;

// === Pattern 4: Traits (Interfaces) ===
// Line 16-22
pub trait Repository {
    fn find_by_id(&self, id: &str) -> Option<User>;
    fn save(&mut self, user: &User) -> Result<(), Error>;
    fn delete(&mut self, id: &str) -> Result<(), Error>;
}

// Line 24-27
pub trait Validator {
    fn validate(&self, item: &Item) -> bool;
}

// === Pattern 5: Enums ===
// Line 30-35
#[derive(Debug, Clone, PartialEq)]
pub enum Status {
    Active,
    Inactive,
    Pending,
}

// Line 37-43
#[derive(Debug, Clone)]
pub enum Priority {
    Low,
    Medium,
    High,
}

// === Pattern 3: Structs ===
// Line 46-52
#[derive(Debug, Clone)]
pub struct User {
    pub id: UserId,
    pub name: String,
    pub email: String,
    pub status: Status,
}

// Line 54-58
pub struct Item {
    pub id: String,
    pub name: String,
    pub value: i32,
}

// Line 60-64
pub struct UserService<R: Repository> {
    repo: R,
    cache: Option<Cache>,
}

// === Pattern 6: Methods (impl blocks) ===
// Line 67-80
impl<R: Repository> UserService<R> {
    pub fn new(repo: R) -> Self {
        Self { repo, cache: None }
    }
    
    pub fn find_by_id(&self, id: &str) -> Option<User> {
        self.repo.find_by_id(id)
    }
    
    pub fn create(&mut self, user: &User) -> Result<(), Error> {
        self.repo.save(user)
    }
}

// Line 82-90
impl Item {
    pub fn new(id: String, name: String, value: i32) -> Self {
        Self { id, name, value }
    }
    
    pub fn is_valid(&self) -> bool {
        self.value > 0
    }
}

// === Pattern 2: Standalone Functions ===
// Line 93-95
pub fn validate_email(email: &str) -> bool {
    email.contains('@')
}

// Line 97-99
pub fn format_user(user: &User) -> String {
    format!("{} <{}>", user.name, user.email)
}

// Line 101-105
pub async fn fetch_user(id: &str) -> Result<User, Error> {
    // Async function
    todo!()
}

// === Pattern: Modules ===
// Line 108-114
pub mod utils {
    pub fn helper() -> bool {
        true
    }
}

mod internal {
    pub(crate) fn private_helper() {}
}

// === Pattern: Macros ===
// Line 117-122
#[macro_export]
macro_rules! log_info {
    ($($arg:tt)*) => {
        println!("[INFO] {}", format!($($arg)*));
    };
}

// === Supporting types ===
// Line 125-126
pub struct Cache;
pub struct Error;
