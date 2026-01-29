/**
 * C++ Parser Test Fixture
 * Tests all symbol extraction capabilities following canonical patterns
 * Line numbers are predictable for automated testing
 */

#pragma once

#include <string>
#include <vector>
#include <memory>
#include <optional>
#include <functional>

// === Pattern 1: Constants ===
// Line 17-20
constexpr int MAX_RETRIES = 3;
constexpr int DEFAULT_TIMEOUT = 30;
constexpr const char* API_VERSION = "1.0.0";
constexpr bool DEBUG_MODE = true;

// Line 22-23
inline constexpr size_t BUFFER_SIZE = 1024;
static const int INTERNAL_LIMIT = 100;

// === Pattern 7: Type Aliases ===
// Line 26-28
using UserId = std::string;
using ItemList = std::vector<class Item>;
using Callback = std::function<void(int)>;

// === Namespace ===
// Line 31
namespace mdemg {

// === Pattern 5: Enums ===
// Line 34-39
enum class Status {
    Active = 1,
    Inactive = 2,
    Pending = 3
};

// Line 41-46
enum class Priority {
    Low = 0,
    Medium = 1,
    High = 2
};

// === Pattern 4: Interfaces (abstract classes) ===
// Line 49-54
class Repository {
public:
    virtual ~Repository() = default;
    virtual std::optional<class User> findById(const UserId& id) = 0;
    virtual bool save(const class User& user) = 0;
    virtual bool remove(const UserId& id) = 0;
};

// Line 56-60
class Validator {
public:
    virtual ~Validator() = default;
    virtual bool validate(const class Item& item) = 0;
};

// === Pattern 3: Classes ===
// Line 63-78
class User {
public:
    User(UserId id, std::string name, std::string email);
    ~User() = default;
    
    // Getters
    const UserId& getId() const { return id_; }
    const std::string& getName() const { return name_; }
    const std::string& getEmail() const { return email_; }
    Status getStatus() const { return status_; }
    
    // Methods
    void deactivate();
    bool isActive() const;
    
private:
    UserId id_;
    std::string name_;
    std::string email_;
    Status status_ = Status::Active;
};

// Line 80-97
class Item {
public:
    Item(std::string id, std::string name, int value);
    ~Item() = default;
    
    // Getters
    const std::string& getId() const { return id_; }
    const std::string& getName() const { return name_; }
    int getValue() const { return value_; }
    
    // Methods
    bool isValid() const;
    double calculateDiscount(double percentage) const;
    
private:
    std::string id_;
    std::string name_;
    int value_;
    Priority priority_ = Priority::Medium;
};

// Line 99-120
template<typename R>
class UserService {
public:
    explicit UserService(std::shared_ptr<R> repository);
    ~UserService() = default;
    
    // Disable copy
    UserService(const UserService&) = delete;
    UserService& operator=(const UserService&) = delete;
    
    // Move operations
    UserService(UserService&&) = default;
    UserService& operator=(UserService&&) = default;
    
    // Methods
    std::optional<User> findById(const UserId& id);
    bool createUser(const User& user);
    bool deleteUser(const UserId& id);
    
private:
    std::shared_ptr<R> repository_;
};

// === Pattern 2: Standalone Functions ===
// Line 123-125
bool validateEmail(const std::string& email);
std::string formatUser(const User& user);
int calculateTotal(const ItemList& items);

// Line 127-129
template<typename T>
T clamp(T value, T min, T max) {
    return std::max(min, std::min(value, max));
}

// === Pattern 3 continued: Inheritance ===
// Line 132-143
class BaseEntity {
public:
    virtual ~BaseEntity() = default;
    virtual std::string getId() const = 0;
    
protected:
    std::string created_at_;
};

class AdminUser : public User, public BaseEntity {
public:
    using User::User;
    std::string getId() const override { return User::getId(); }
};

} // namespace mdemg

// === Implementation (normally in .cpp file) ===
// Line 148-153
namespace mdemg {

User::User(UserId id, std::string name, std::string email)
    : id_(std::move(id)), name_(std::move(name)), email_(std::move(email)) {}

// Line 155-157
void User::deactivate() {
    status_ = Status::Inactive;
}

// Line 159-161
bool User::isActive() const {
    return status_ == Status::Active;
}

// Line 163-166
Item::Item(std::string id, std::string name, int value)
    : id_(std::move(id)), name_(std::move(name)), value_(value) {}

// Line 168-170
bool Item::isValid() const {
    return value_ > 0;
}

// Line 172-174
double Item::calculateDiscount(double percentage) const {
    return value_ * (1.0 - percentage / 100.0);
}

// Line 176-178
bool validateEmail(const std::string& email) {
    return email.find('@') != std::string::npos;
}

// Line 180-182
std::string formatUser(const User& user) {
    return user.getName() + " <" + user.getEmail() + ">";
}

// Line 184-190
int calculateTotal(const ItemList& items) {
    int total = 0;
    for (const auto& item : items) {
        total += item.getValue();
    }
    return total;
}

} // namespace mdemg
