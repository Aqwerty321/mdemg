// C# Parser Test Fixture
// Tests all symbol extraction capabilities following canonical patterns
// Line numbers are predictable for automated testing
using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Linq;

namespace TestNamespace.Models
{
    // === Pattern 1: Constants ===
    public static class AppConstants
    {
        public const int MaxRetries = 3;
        public const string ApiVersion = "2.0.0";
        public const bool DebugMode = false;
        public static readonly TimeSpan DefaultTimeout = TimeSpan.FromSeconds(30);
        public static readonly string ConnectionString = "Server=localhost";
        internal const double TaxRate = 0.08;
    }

    // === Pattern 2: Enum ===
    public enum Status
    {
        Active,
        Inactive,
        Pending,
        Deleted
    }

    // === Pattern 3: Interface ===
    public interface IRepository<T>
    {
        T FindById(int id);
        IEnumerable<T> GetAll();
        void Save(T entity);
        void Delete(int id);
    }

    // === Pattern 4: Second interface ===
    internal interface IValidator
    {
        bool Validate(object item);
        List<string> GetErrors();
    }

    // === Pattern 5: Class with fields, properties, methods, constructor ===
    [Serializable]
    public class User
    {
        private string _id;
        private string _name;
        private string _email;

        public string Id { get; set; }
        public string Name { get; set; }
        public string Email { get; private set; }
        public Status CurrentStatus { get; protected set; }
        public DateTime CreatedAt { get; init; }

        public User(string id, string name, string email)
        {
            _id = id;
            _name = name;
            _email = email;
            CurrentStatus = Status.Active;
            CreatedAt = DateTime.UtcNow;
        }

        public void Deactivate()
        {
            CurrentStatus = Status.Inactive;
        }

        public bool IsActive()
        {
            return CurrentStatus == Status.Active;
        }

        private void ValidateEmail()
        {
            if (string.IsNullOrEmpty(_email))
                throw new ArgumentException("Email required");
        }

        protected virtual string FormatDisplay()
        {
            return $"{_name} <{_email}>";
        }
    }

    // === Pattern 6: Static class with static methods ===
    public static class StringUtils
    {
        public static bool IsNullOrEmpty(string value)
        {
            return string.IsNullOrEmpty(value);
        }

        public static string Truncate(string value, int maxLength)
        {
            if (value == null) return null;
            return value.Length <= maxLength ? value : value.Substring(0, maxLength);
        }

        internal static int CountWords(string text)
        {
            return text.Split(' ', StringSplitOptions.RemoveEmptyEntries).Length;
        }
    }

    // === Pattern 7: Struct ===
    public struct Coordinate
    {
        public double Latitude { get; set; }
        public double Longitude { get; set; }

        public Coordinate(double lat, double lon)
        {
            Latitude = lat;
            Longitude = lon;
        }

        public double DistanceTo(Coordinate other)
        {
            return Math.Sqrt(Math.Pow(Latitude - other.Latitude, 2) + Math.Pow(Longitude - other.Longitude, 2));
        }
    }

    // === Pattern 8: Record type ===
    public record UserDto(string Id, string Name, string Email, Status Status);

    // === Pattern 9: Delegate ===
    public delegate void EventHandler<TArgs>(object sender, TArgs args);

    // === Pattern 10: Class with nested class and inheritance ===
    [HttpController]
    public class UserService : IRepository<User>
    {
        private readonly IValidator _validator;
        private static readonly string ServiceName = "UserService";

        public UserService(IValidator validator)
        {
            _validator = validator;
        }

        [HttpGet]
        public User FindById(int id)
        {
            return new User(id.ToString(), "unknown", "unknown@test.com");
        }

        public IEnumerable<User> GetAll()
        {
            return new List<User>();
        }

        [HttpPost]
        public void Save(User entity)
        {
            if (!_validator.Validate(entity))
                throw new InvalidOperationException("Validation failed");
        }

        [HttpDelete]
        public void Delete(int id)
        {
            // Delete implementation
        }

        public async Task<User> FindByIdAsync(int id)
        {
            await Task.Delay(10);
            return FindById(id);
        }

        private bool ValidateInput(string input)
        {
            return !string.IsNullOrWhiteSpace(input);
        }

        // Nested class
        public class UserComparer : IComparer<User>
        {
            public int Compare(User x, User y)
            {
                return string.Compare(x.Name, y.Name, StringComparison.Ordinal);
            }
        }
    }

    // === Pattern 11: Abstract class ===
    public abstract class BaseEntity
    {
        public int Id { get; set; }
        public DateTime CreatedAt { get; set; }
        public DateTime? UpdatedAt { get; set; }

        public abstract void Validate();

        public virtual void Touch()
        {
            UpdatedAt = DateTime.UtcNow;
        }
    }
}
