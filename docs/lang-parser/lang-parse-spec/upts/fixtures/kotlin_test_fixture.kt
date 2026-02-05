/**
 * Kotlin Parser Test Fixture
 * Tests all symbol extraction capabilities for the Kotlin language parser
 * Line numbers are predictable for automated UPTS testing
 */
package com.mdemg.testfixture

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.Deferred
import java.time.Instant
import java.util.UUID

// === Pattern 1: Typealias ===
typealias UserId = String
typealias EventHandler = (String, Int) -> Boolean

// === Pattern 2: Data class ===
data class User(
    val id: UserId,
    val name: String,
    val email: String,
    val status: Status = Status.ACTIVE,
    val createdAt: Instant = Instant.now()
)

// === Pattern 3: Sealed class with subclasses ===
sealed class Result<out T> {
    data class Success<T>(val data: T) : Result<T>()
    data class Error(val message: String, val code: Int) : Result<Nothing>()
    object Loading : Result<Nothing>()
}

// === Pattern 4: Enum class ===
enum class Status(val code: Int) {
    ACTIVE(1),
    INACTIVE(2),
    PENDING(3),
    SUSPENDED(4);

    fun isUsable(): Boolean = this == ACTIVE || this == PENDING
}

// === Pattern 5: Interface ===
interface Repository<T, ID> {
    fun findById(id: ID): T?
    fun save(entity: T): T
    fun delete(id: ID)
    fun findAll(): List<T>
}

// === Pattern 6: Interface with default method ===
interface Auditable {
    val createdAt: Instant
    val updatedAt: Instant

    fun auditInfo(): String = "Created: $createdAt, Updated: $updatedAt"
}

// === Pattern 7: Abstract class ===
abstract class BaseEntity(val id: String) {
    abstract fun validate(): Boolean
    abstract fun toMap(): Map<String, Any>

    open fun describe(): String = "Entity($id)"
}

// === Pattern 8: Object (singleton) ===
object AppConfig {
    const val MAX_RETRIES = 3
    const val DEFAULT_TIMEOUT = 30_000L
    const val API_VERSION = "2.1.0"

    private val listeners = mutableListOf<EventHandler>()

    fun register(handler: EventHandler) {
        listeners.add(handler)
    }
}

// === Pattern 9: Class with companion object ===
class UserService(private val repository: Repository<User, UserId>) {

    companion object {
        const val MAX_PAGE_SIZE = 100
        const val DEFAULT_PAGE_SIZE = 20

        @JvmStatic
        fun create(repository: Repository<User, UserId>): UserService {
            return UserService(repository)
        }
    }

    fun findById(id: UserId): User? {
        return repository.findById(id)
    }

    @Deprecated("Use createUser(name, email, role) instead")
    fun createUser(name: String, email: String): User {
        val user = User(id = UUID.randomUUID().toString(), name = name, email = email)
        return repository.save(user)
    }

    internal fun validateEmail(email: String): Boolean {
        return email.contains("@") && email.contains(".")
    }

    private fun generateId(): UserId {
        return UUID.randomUUID().toString()
    }
}

// === Pattern 10: Enum class with methods ===
enum class Priority(val level: Int) {
    LOW(1),
    MEDIUM(5),
    HIGH(10),
    CRITICAL(99);

    fun isUrgent(): Boolean = level >= 10
}

// === Pattern 11: Extension functions (top-level) ===
fun String.isValidEmail(): Boolean {
    return contains("@") && contains(".")
}

fun List<User>.activeUsers(): List<User> {
    return filter { it.status == Status.ACTIVE }
}

// === Pattern 12: Top-level functions ===
fun calculateTotal(items: List<Int>): Int {
    return items.sum()
}

@JvmStatic
fun formatUser(user: User): String {
    return "${user.name} <${user.email}>"
}

// === Pattern 13: Top-level val/var constants ===
val DEFAULT_LOCALE = "en_US"
var currentEnvironment = "development"

// === Pattern 14: Class implementing interface ===
class InMemoryUserRepository : Repository<User, UserId>, Auditable {
    override val createdAt: Instant = Instant.now()
    override val updatedAt: Instant = Instant.now()

    private val store = mutableMapOf<UserId, User>()

    override fun findById(id: UserId): User? = store[id]

    override fun save(entity: User): User {
        store[entity.id] = entity
        return entity
    }

    override fun delete(id: UserId) {
        store.remove(id)
    }

    override fun findAll(): List<User> = store.values.toList()

    internal fun clear() {
        store.clear()
    }
}
