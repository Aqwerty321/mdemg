/**
 * CUDA Parser Test Fixture
 * Tests GPU kernel and device function extraction
 * Line numbers are predictable for automated testing
 */

#pragma once

#include <cuda_runtime.h>
#include <device_launch_parameters.h>

// === Pattern 1: Constants ===
// Line 14-17
#define BLOCK_SIZE 256
#define MAX_THREADS 1024
#define WARP_SIZE 32
#define SHARED_MEM_SIZE 48 * 1024

// Line 19-21
__constant__ float d_multiplier = 1.0f;
__constant__ int d_offset = 0;
__constant__ float d_coefficients[16];

// Line 23-25
constexpr int NUM_STREAMS = 4;
constexpr size_t BUFFER_SIZE = 1024 * 1024;
static const int HOST_CONSTANT = 42;

// === Pattern: Device memory declarations ===
// Line 28-30
__device__ float* d_input;
__device__ float* d_output;
__device__ int d_counter;

// === Pattern 3: Structures for GPU ===
// Line 33-39
struct __align__(16) Vector3 {
    float x, y, z, w;
    
    __host__ __device__ float magnitude() const {
        return sqrtf(x*x + y*y + z*z);
    }
};

// Line 41-47
struct Particle {
    Vector3 position;
    Vector3 velocity;
    float mass;
    int id;
};

// Line 49-55
struct SimulationParams {
    int numParticles;
    float timestep;
    float damping;
    Vector3 gravity;
};

// === Pattern: Device functions ===
// Line 58-61
__device__ __forceinline__ float atomicAddFloat(float* address, float val) {
    return atomicAdd(address, val);
}

// Line 63-67
__device__ float computeDistance(const Vector3& a, const Vector3& b) {
    float dx = a.x - b.x;
    float dy = a.y - b.y;
    float dz = a.z - b.z;
    return sqrtf(dx*dx + dy*dy + dz*dz);
}

// Line 69-76
__device__ void updateParticle(Particle& p, const SimulationParams& params) {
    p.velocity.x += params.gravity.x * params.timestep;
    p.velocity.y += params.gravity.y * params.timestep;
    p.velocity.z += params.gravity.z * params.timestep;
    p.position.x += p.velocity.x * params.timestep;
    p.position.y += p.velocity.y * params.timestep;
    p.position.z += p.velocity.z * params.timestep;
}

// === Pattern: Global kernels ===
// Line 79-92
__global__ void vectorAdd(const float* a, const float* b, float* c, int n) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    
    if (idx < n) {
        c[idx] = a[idx] + b[idx];
    }
}

// Line 94-106
__global__ void matrixMultiply(
    const float* A, const float* B, float* C,
    int M, int N, int K
) {
    __shared__ float sharedA[BLOCK_SIZE][BLOCK_SIZE];
    __shared__ float sharedB[BLOCK_SIZE][BLOCK_SIZE];
    
    int row = blockIdx.y * blockDim.y + threadIdx.y;
    int col = blockIdx.x * blockDim.x + threadIdx.x;
    
    float sum = 0.0f;
    // ... computation
    C[row * N + col] = sum;
}

// Line 108-124
__global__ void particleSimulation(
    Particle* particles,
    const SimulationParams params,
    int numIterations
) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    
    if (idx < params.numParticles) {
        Particle& p = particles[idx];
        
        for (int i = 0; i < numIterations; i++) {
            updateParticle(p, params);
        }
    }
}

// Line 126-141
__global__ void reduceSum(const float* input, float* output, int n) {
    __shared__ float sdata[BLOCK_SIZE];
    
    int tid = threadIdx.x;
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    
    sdata[tid] = (idx < n) ? input[idx] : 0.0f;
    __syncthreads();
    
    // Reduction
    for (int s = blockDim.x / 2; s > 0; s >>= 1) {
        if (tid < s) {
            sdata[tid] += sdata[tid + s];
        }
        __syncthreads();
    }
    
    if (tid == 0) output[blockIdx.x] = sdata[0];
}

// === Pattern 2: Host functions ===
// Line 144-153
void launchVectorAdd(const float* a, const float* b, float* c, int n) {
    int blockSize = BLOCK_SIZE;
    int numBlocks = (n + blockSize - 1) / blockSize;
    
    vectorAdd<<<numBlocks, blockSize>>>(a, b, c, n);
    cudaDeviceSynchronize();
}

// Line 155-168
cudaError_t allocateDeviceMemory(void** ptr, size_t size) {
    return cudaMalloc(ptr, size);
}

void freeDeviceMemory(void* ptr) {
    cudaFree(ptr);
}

// Line 170-178
template<typename T>
void copyToDevice(T* d_ptr, const T* h_ptr, size_t count) {
    cudaMemcpy(d_ptr, h_ptr, count * sizeof(T), cudaMemcpyHostToDevice);
}

template<typename T>
void copyToHost(T* h_ptr, const T* d_ptr, size_t count) {
    cudaMemcpy(h_ptr, d_ptr, count * sizeof(T), cudaMemcpyDeviceToHost);
}

// === Compute capability guards ===
// Line 181-187
#if __CUDA_ARCH__ >= 700
__device__ void tensorCoreOperation() {
    // Volta+ tensor core operations
}
#endif

#if __CUDA_ARCH__ >= 800
__device__ void asyncCopyOperation() {
    // Ampere+ async copy
}
#endif
