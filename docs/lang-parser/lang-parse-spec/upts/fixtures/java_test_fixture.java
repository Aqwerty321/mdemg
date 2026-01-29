/**
 * Java Parser Test Fixture
 * Tests all symbol extraction capabilities following canonical patterns
 * Line numbers are predictable for automated testing
 */
package com.mdemg.testfixture;

import java.util.List;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import java.time.Instant;

// === Pattern 1: Constants ===
// Line 15-18
public class Constants {
    public static final int MAX_RETRIES = 3;
    public static final int DEFAULT_TIMEOUT = 30;
    public static final String API_VERSION = "1.0.0";
    public static final boolean DEBUG_MODE = true;
}

// === Pattern 5: Enums ===
// Line 22-27
public enum Status {
    ACTIVE(1),
    INACTIVE(2),
    PENDING(3);
    
    private final int value;
    Status(int value) { this.value = value; }
}

// Line 30-35
public enum Priority {
    LOW("low"),
    MEDIUM("medium"),
    HIGH("high");
    
    private final String label;
    Priority(String label) { this.label = label; }
}

// === Pattern 4: Interfaces ===
// Line 38-42
public interface Repository<T, ID> {
    Optional<T> findById(ID id);
    T save(T entity);
    void delete(ID id);
}

// Line 44-47
public interface Validator<T> {
    boolean validate(T item);
    List<String> getErrors();
}

// Line 49-52
public interface UserRepository extends Repository<User, String> {
    List<User> findByStatus(Status status);
    Optional<User> findByEmail(String email);
}

// === Pattern 3: Classes ===
// Line 55-77
public class User {
    private String id;
    private String name;
    private String email;
    private Status status;
    private Instant createdAt;
    
    public User(String id, String name, String email) {
        this.id = id;
        this.name = name;
        this.email = email;
        this.status = Status.ACTIVE;
        this.createdAt = Instant.now();
    }
    
    // Getters
    public String getId() { return id; }
    public String getName() { return name; }
    public String getEmail() { return email; }
    public Status getStatus() { return status; }
    
    // Methods
    public void deactivate() {
        this.status = Status.INACTIVE;
    }
    
    public boolean isActive() {
        return this.status == Status.ACTIVE;
    }
}

// Line 79-102
public class Item {
    private String id;
    private String name;
    private int value;
    private Priority priority;
    
    public Item(String id, String name, int value) {
        this.id = id;
        this.name = name;
        this.value = value;
        this.priority = Priority.MEDIUM;
    }
    
    public String getId() { return id; }
    public String getName() { return name; }
    public int getValue() { return value; }
    
    public boolean isValid() {
        return value > 0 && name != null && !name.isEmpty();
    }
    
    public double calculateDiscount(double percentage) {
        return value * (1.0 - percentage / 100.0);
    }
}

// Line 104-131
@Service
public class UserService {
    private final UserRepository repository;
    private final Logger logger;
    
    @Autowired
    public UserService(UserRepository repository) {
        this.repository = repository;
        this.logger = LoggerFactory.getLogger(UserService.class);
    }
    
    public Optional<User> findById(String id) {
        return repository.findById(id);
    }
    
    @Transactional
    public User createUser(String name, String email) {
        User user = new User(generateId(), name, email);
        return repository.save(user);
    }
    
    @Async
    public CompletableFuture<User> createUserAsync(String name, String email) {
        return CompletableFuture.supplyAsync(() -> createUser(name, email));
    }
    
    public void deleteUser(String id) {
        repository.delete(id);
    }
    
    private String generateId() {
        return java.util.UUID.randomUUID().toString();
    }
}

// === Pattern 3 continued: Abstract class and inheritance ===
// Line 134-145
public abstract class BaseEntity {
    protected String id;
    protected Instant createdAt;
    protected Instant updatedAt;
    
    public abstract String getId();
    
    public void touch() {
        this.updatedAt = Instant.now();
    }
}

// Line 147-159
public class AdminUser extends User implements Comparable<AdminUser> {
    private List<String> permissions;
    
    public AdminUser(String id, String name, String email) {
        super(id, name, email);
        this.permissions = new ArrayList<>();
    }
    
    public void addPermission(String permission) {
        permissions.add(permission);
    }
    
    @Override
    public int compareTo(AdminUser other) {
        return this.getName().compareTo(other.getName());
    }
}

// === Pattern 2: Standalone utility methods ===
// Line 162-173
public class Utils {
    
    public static boolean validateEmail(String email) {
        return email != null && email.contains("@");
    }
    
    public static String formatUser(User user) {
        return String.format("%s <%s>", user.getName(), user.getEmail());
    }
    
    public static int calculateTotal(List<Item> items) {
        return items.stream().mapToInt(Item::getValue).sum();
    }
}

// === Pattern: Generic class ===
// Line 176-192
public class Cache<K, V> {
    private final Map<K, V> store;
    private final int maxSize;
    
    public Cache(int maxSize) {
        this.store = new ConcurrentHashMap<>();
        this.maxSize = maxSize;
    }
    
    public void put(K key, V value) {
        if (store.size() < maxSize) {
            store.put(key, value);
        }
    }
    
    public Optional<V> get(K key) {
        return Optional.ofNullable(store.get(key));
    }
    
    public void clear() {
        store.clear();
    }
}

// === Pattern: Record (Java 14+) ===
// Line 195-196
public record UserDto(String id, String name, String email, Status status) {}
public record ItemDto(String id, String name, int value) {}
