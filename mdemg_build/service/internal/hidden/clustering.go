package hidden

import (
	"math"
)

// DBSCAN implements density-based spatial clustering of applications with noise.
// It uses cosine distance (1 - cosine_similarity) as the distance metric.
//
// Parameters:
//   - points: slice of embeddings to cluster
//   - eps: maximum distance for two points to be considered neighbors
//   - minSamples: minimum number of points to form a dense region (cluster)
//
// Returns:
//   - labels: cluster assignment for each point (-1 = noise)
func DBSCAN(points [][]float64, eps float64, minSamples int) []int {
	n := len(points)
	if n == 0 {
		return nil
	}

	labels := make([]int, n)
	for i := range labels {
		labels[i] = -2 // undefined
	}

	clusterID := 0

	for i := 0; i < n; i++ {
		if labels[i] != -2 {
			continue // already processed
		}

		neighbors := regionQuery(points, i, eps)
		if len(neighbors) < minSamples {
			labels[i] = -1 // noise
			continue
		}

		// Start a new cluster
		labels[i] = clusterID
		seedSet := make([]int, len(neighbors))
		copy(seedSet, neighbors)

		for j := 0; j < len(seedSet); j++ {
			q := seedSet[j]
			if labels[q] == -1 {
				labels[q] = clusterID // change noise to border point
			}
			if labels[q] != -2 {
				continue // already processed
			}

			labels[q] = clusterID

			qNeighbors := regionQuery(points, q, eps)
			if len(qNeighbors) >= minSamples {
				// Add new neighbors to seed set
				for _, neighbor := range qNeighbors {
					if !contains(seedSet, neighbor) {
						seedSet = append(seedSet, neighbor)
					}
				}
			}
		}

		clusterID++
	}

	return labels
}

// regionQuery finds all points within eps distance of point at index
func regionQuery(points [][]float64, index int, eps float64) []int {
	var neighbors []int
	point := points[index]

	for i, other := range points {
		if i == index {
			continue
		}
		dist := cosineDistance(point, other)
		if dist <= eps {
			neighbors = append(neighbors, i)
		}
	}

	return neighbors
}

// cosineDistance computes 1 - cosine_similarity between two vectors
func cosineDistance(a, b []float64) float64 {
	sim := cosineSimilarity(a, b)
	return 1.0 - sim
}

// cosineSimilarity computes the cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// contains checks if a slice contains a value
func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// ComputeCentroid calculates the centroid (element-wise mean) of multiple embeddings
func ComputeCentroid(embeddings [][]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}

	dim := len(embeddings[0])
	if dim == 0 {
		return nil
	}

	centroid := make([]float64, dim)
	count := 0

	for _, emb := range embeddings {
		if len(emb) != dim {
			continue // skip mismatched dimensions
		}
		for i, v := range emb {
			centroid[i] += v
		}
		count++
	}

	if count == 0 {
		return nil
	}

	for i := range centroid {
		centroid[i] /= float64(count)
	}

	return centroid
}

// NormalizeVector normalizes a vector to unit length
func NormalizeVector(v []float64) []float64 {
	if len(v) == 0 {
		return nil
	}

	var norm float64
	for _, val := range v {
		norm += val * val
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return v
	}

	result := make([]float64, len(v))
	for i, val := range v {
		result[i] = val / norm
	}

	return result
}

// GroupByCluster groups base nodes by their cluster labels
func GroupByCluster(nodes []BaseNode, labels []int) (map[int][]BaseNode, []BaseNode) {
	clusters := make(map[int][]BaseNode)
	var noise []BaseNode

	for i, label := range labels {
		if label == -1 {
			noise = append(noise, nodes[i])
		} else {
			clusters[label] = append(clusters[label], nodes[i])
		}
	}

	return clusters, noise
}
