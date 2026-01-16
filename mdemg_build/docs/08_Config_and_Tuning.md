# Config + Tuning Knobs

Put these in config, not code.

## Activation
- `activation_steps_T` (default 3)
- `activation_step_decay_lambda` (default 0.15)
- `activation_min_threshold` (default 0.20) — below this ignore nodes for learning

## Edge weight model
- base weight clamp: `w_min`, `w_max`
- dimension coefficients:
  - `alpha_semantic`
  - `beta_temporal`
  - `gamma_coactivation`
  - `delta_causal`
- `recency_rho` for exp(-rho * age)

## Learning (Hebbian)
- `eta_learning_rate` (default 0.02)
- `mu_regularization` (default 0.01)
- `coactivation_edge_create_weight` (default 0.1)
- `coactivation_update_cap_per_request` (default 200 edges)

## Decay + pruning
- `default_decay_rate` per edge type
- `prune_weight_threshold`
- `prune_min_evidence`
- `prune_edge_age_days`

## Consolidation
- `cluster_min_size` (default 3)
- `cluster_max_size` (default 15)
- `cluster_min_density`
- `cluster_evidence_threshold`
- `consolidation_interval_hours`

## Retrieval scoring weights
- `alpha_vector`
- `beta_activation`
- `gamma_recency`
- `delta_confidence`
- `kappa_redundancy`
- `phi_hub_penalty`

## Guardrails
- `max_neighbors_per_node` during expansion
- `max_total_edges_fetched`
- `allowed_relationship_types` per retrieval mode
