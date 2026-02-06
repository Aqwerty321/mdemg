-- Lua Test Fixture for UPTS Validation
-- Covers all canonical patterns for Lua symbol extraction
-- Line numbers are predictable for automated testing

-- ====================
-- P1: CONSTANTS (local)
-- ====================
local MAX_BUFFER_SIZE = 4096
local VERSION = "1.0.0"
local DEBUG_MODE = false

-- ====================
-- P2: CONSTANTS (global)
-- ====================
PI = 3.14159
DEFAULT_TIMEOUT = 30000
APP_NAME = "TestApp"

-- ====================
-- P3: GLOBAL FUNCTIONS
-- ====================
function calculateSum(a, b)
    return a + b
end

function multiply(x, y)
    return x * y
end

function processData(data, options)
    if not data then return nil end
    return data
end

-- ====================
-- P4: LOCAL FUNCTIONS
-- ====================
local function privateHelper()
    return true
end

local function validateInput(input)
    return input ~= nil and input ~= ""
end

-- ====================
-- P5: CLASS-LIKE TABLES
-- ====================
User = {}
User.__index = User

function User:new(id, name)
    local self = setmetatable({}, User)
    self.id = id
    self.name = name
    self.active = true
    return self
end

function User:getName()
    return self.name
end

function User:setName(name)
    self.name = name
end

function User:deactivate()
    self.active = false
end

function User:isActive()
    return self.active
end

-- Another class
Repository = {}
Repository.__index = Repository

function Repository:new()
    local self = setmetatable({}, Repository)
    self.items = {}
    return self
end

function Repository:add(item)
    table.insert(self.items, item)
end

function Repository:find(id)
    for _, item in ipairs(self.items) do
        if item.id == id then
            return item
        end
    end
    return nil
end

function Repository:count()
    return #self.items
end

function Repository:clear()
    self.items = {}
end

-- ====================
-- P6: ENUM-LIKE TABLES
-- ====================
Status = {
    ACTIVE = "active",
    INACTIVE = "inactive",
    PENDING = "pending",
    DELETED = "deleted"
}

Priority = {
    LOW = 1,
    MEDIUM = 2,
    HIGH = 3,
    CRITICAL = 4
}

-- ====================
-- P7: MODULE PATTERN
-- ====================
local M = {}

function M.init()
    return true
end

function M.process(data)
    return data
end

function M.cleanup()
    -- cleanup logic
end

return M
