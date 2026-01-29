/**
 * C Parser Test Fixture
 * Tests all symbol extraction capabilities following canonical patterns
 * Line numbers are predictable for automated testing
 */

#ifndef TEST_FIXTURE_H
#define TEST_FIXTURE_H

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

// === Pattern 1: Constants (macros) ===
// Line 17-20
#define MAX_RETRIES 3
#define DEFAULT_TIMEOUT 30
#define API_VERSION "1.0.0"
#define DEBUG_MODE 1

// Line 22-25
#define BUFFER_SIZE 1024
#define MIN(a, b) ((a) < (b) ? (a) : (b))
#define MAX(a, b) ((a) > (b) ? (a) : (b))
#define STRINGIFY(x) #x

// === Pattern 7: Type Aliases ===
// Line 28-30
typedef unsigned int UserId;
typedef char* String;
typedef struct Item* ItemList;

// === Pattern 5: Enums ===
// Line 33-38
typedef enum {
    STATUS_ACTIVE = 1,
    STATUS_INACTIVE = 2,
    STATUS_PENDING = 3
} Status;

// Line 40-45
typedef enum Priority {
    PRIORITY_LOW = 0,
    PRIORITY_MEDIUM = 1,
    PRIORITY_HIGH = 2
} Priority;

// === Pattern 3: Structs ===
// Line 48-54
typedef struct User {
    UserId id;
    String name;
    String email;
    Status status;
    time_t created_at;
} User;

// Line 56-61
typedef struct Item {
    char id[36];
    char name[256];
    int value;
    Priority priority;
} Item;

// Line 63-67
typedef struct UserService {
    void* repository;
    void* logger;
    int max_retries;
} UserService;

// === Pattern 4: Forward declarations / opaque types ===
// Line 70-71
struct InternalCache;
typedef struct InternalCache InternalCache;

// === Pattern 2: Function declarations ===
// Line 74-76
User* user_create(UserId id, const char* name, const char* email);
void user_destroy(User* user);
bool user_is_active(const User* user);

// Line 78-80
Item* item_create(const char* name, int value);
void item_destroy(Item* item);
bool item_is_valid(const Item* item);

// Line 82-85
UserService* user_service_create(void* repository, void* logger);
void user_service_destroy(UserService* service);
User* user_service_find_by_id(UserService* service, UserId id);
int user_service_create_user(UserService* service, const User* user);

// Line 87-89
bool validate_email(const char* email);
char* format_user(const User* user);
int calculate_total(const Item* items, size_t count);

// === Pattern: Static inline functions ===
// Line 92-94
static inline bool is_valid_id(UserId id) {
    return id > 0;
}

// Line 96-98
static inline int clamp(int value, int min, int max) {
    return MIN(MAX(value, min), max);
}

#endif /* TEST_FIXTURE_H */

// === Implementation (normally in .c file) ===
// Line 103-108
User* user_create(UserId id, const char* name, const char* email) {
    User* user = malloc(sizeof(User));
    if (!user) return NULL;
    user->id = id;
    user->status = STATUS_ACTIVE;
    return user;
}

// Line 110-113
void user_destroy(User* user) {
    if (user) free(user);
}

// Line 115-117
bool user_is_active(const User* user) {
    return user && user->status == STATUS_ACTIVE;
}

// Line 119-122
bool validate_email(const char* email) {
    return email && strchr(email, '@') != NULL;
}

// Line 124-130
char* format_user(const User* user) {
    if (!user) return NULL;
    char* result = malloc(512);
    if (!result) return NULL;
    snprintf(result, 512, "%s <%s>", user->name, user->email);
    return result;
}

// Line 132-139
int calculate_total(const Item* items, size_t count) {
    int total = 0;
    for (size_t i = 0; i < count; i++) {
        total += items[i].value;
    }
    return total;
}
