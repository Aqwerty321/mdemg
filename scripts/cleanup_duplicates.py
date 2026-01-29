#!/usr/bin/env python3
"""
MDEMG Data Quality Cleanup - Phase 2a-c

Archives duplicate nodes, stop-word nodes, and cleans orphaned edges.
All operations are reversible via is_archived flag.

Usage:
    python3 scripts/cleanup_duplicates.py [--dry-run] [--space-id SPACE_ID]
"""

import argparse
import sys
from neo4j import GraphDatabase

NEO4J_URI = "bolt://localhost:7687"
NEO4J_USER = "neo4j"
NEO4J_PASS = "testpassword"

STOP_WORDS = {"for", "and", "is", "the", "of", "to", "in", "a", "an", "or", "not", "with", "as", "at", "by"}


def archive_duplicates(session, space_id: str, dry_run: bool) -> int:
    """Phase 2a: Archive duplicate nodes (keep highest-confidence per name group)."""
    # Find (space_id, name) groups with >5 duplicates
    query = """
    MATCH (n:MemoryNode {space_id: $spaceId})
    WHERE n.is_archived IS NULL OR n.is_archived = false
    WITH n.name AS name, collect(n) AS nodes, count(n) AS cnt
    WHERE cnt > 5
    RETURN name, cnt
    ORDER BY cnt DESC
    """
    result = session.run(query, spaceId=space_id)
    groups = [(r["name"], r["cnt"]) for r in result]

    if not groups:
        print("  No duplicate groups found (>5 per name).")
        return 0

    print(f"  Found {len(groups)} duplicate groups:")
    for name, cnt in groups[:10]:
        print(f"    '{name}': {cnt} copies")
    if len(groups) > 10:
        print(f"    ... and {len(groups) - 10} more groups")

    if dry_run:
        total = sum(cnt - 1 for _, cnt in groups)
        print(f"  [DRY RUN] Would archive {total} duplicate nodes")
        return total

    # Archive all but highest-confidence node per group
    archive_query = """
    MATCH (n:MemoryNode {space_id: $spaceId})
    WHERE n.is_archived IS NULL OR n.is_archived = false
    WITH n.name AS name, collect(n) AS nodes, count(n) AS cnt
    WHERE cnt > 5
    WITH name, nodes, cnt,
         reduce(best = nodes[0], n IN nodes |
           CASE WHEN coalesce(n.confidence, 0) > coalesce(best.confidence, 0) THEN n ELSE best END
         ) AS keeper
    UNWIND nodes AS n
    WITH n, keeper WHERE n <> keeper
    SET n.is_archived = true, n.archived_reason = 'duplicate_cleanup'
    RETURN count(n) AS archived
    """
    result = session.run(archive_query, spaceId=space_id)
    archived = result.single()["archived"]
    print(f"  Archived {archived} duplicate nodes")
    return archived


def archive_stop_words(session, space_id: str, dry_run: bool) -> int:
    """Phase 2b: Archive stop-word nodes."""
    query = """
    MATCH (n:MemoryNode {space_id: $spaceId})
    WHERE (n.is_archived IS NULL OR n.is_archived = false)
      AND toLower(trim(n.name)) IN $stopWords
    RETURN count(n) AS cnt
    """
    result = session.run(query, spaceId=space_id, stopWords=list(STOP_WORDS))
    cnt = result.single()["cnt"]

    if cnt == 0:
        print("  No stop-word nodes found.")
        return 0

    if dry_run:
        print(f"  [DRY RUN] Would archive {cnt} stop-word nodes")
        return cnt

    archive_query = """
    MATCH (n:MemoryNode {space_id: $spaceId})
    WHERE (n.is_archived IS NULL OR n.is_archived = false)
      AND toLower(trim(n.name)) IN $stopWords
    SET n.is_archived = true, n.archived_reason = 'stop_word_cleanup'
    RETURN count(n) AS archived
    """
    result = session.run(archive_query, spaceId=space_id, stopWords=list(STOP_WORDS))
    archived = result.single()["archived"]
    print(f"  Archived {archived} stop-word nodes")
    return archived


def clean_orphaned_edges(session, space_id: str, dry_run: bool) -> int:
    """Phase 2c: Delete CO_ACTIVATED_WITH edges where either endpoint is archived."""
    query = """
    MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->(b:MemoryNode {space_id: $spaceId})
    WHERE a.is_archived = true OR b.is_archived = true
    RETURN count(r) AS cnt
    """
    result = session.run(query, spaceId=space_id)
    cnt = result.single()["cnt"]

    if cnt == 0:
        print("  No orphaned edges found.")
        return 0

    if dry_run:
        print(f"  [DRY RUN] Would delete {cnt} orphaned edges")
        return cnt

    delete_query = """
    MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->(b:MemoryNode {space_id: $spaceId})
    WHERE a.is_archived = true OR b.is_archived = true
    DELETE r
    RETURN count(r) AS deleted
    """
    result = session.run(delete_query, spaceId=space_id)
    deleted = result.single()["deleted"]
    print(f"  Deleted {deleted} orphaned edges")
    return deleted


def backfill_dim_semantic(session, space_id: str, dry_run: bool) -> int:
    """Phase 1d: Backfill dim_semantic on existing edges based on path-prefix similarity."""
    query = """
    MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->(b:MemoryNode {space_id: $spaceId})
    WHERE r.dim_semantic IS NULL
    WITH r, a.path AS pathA, b.path AS pathB,
         CASE
           WHEN a.path IS NULL OR b.path IS NULL THEN 0.0
           WHEN replace(a.path, split(a.path, '/')[-1], '') = replace(b.path, split(b.path, '/')[-1], '') THEN 0.8
           WHEN split(a.path, '/')[0] = split(b.path, '/')[0] THEN 0.5
           ELSE 0.0
         END AS pathSim
    RETURN count(r) AS cnt, avg(pathSim) AS avgSim
    """
    result = session.run(query, spaceId=space_id)
    rec = result.single()
    cnt = rec["cnt"]
    avg_sim = rec["avgSim"] or 0

    if cnt == 0:
        print("  No edges need dim_semantic backfill.")
        return 0

    print(f"  Found {cnt} edges missing dim_semantic (avg pathSim={avg_sim:.3f})")

    if dry_run:
        print(f"  [DRY RUN] Would backfill dim_semantic on {cnt} edges")
        return cnt

    update_query = """
    MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->(b:MemoryNode {space_id: $spaceId})
    WHERE r.dim_semantic IS NULL
    WITH r,
         CASE
           WHEN a.path IS NULL OR b.path IS NULL THEN 0.0
           WHEN replace(a.path, split(a.path, '/')[-1], '') = replace(b.path, split(b.path, '/')[-1], '') THEN 0.8
           WHEN split(a.path, '/')[0] = split(b.path, '/')[0] THEN 0.5
           ELSE 0.0
         END AS pathSim
    SET r.dim_semantic = pathSim
    RETURN count(r) AS updated
    """
    result = session.run(update_query, spaceId=space_id)
    updated = result.single()["updated"]
    print(f"  Backfilled dim_semantic on {updated} edges")
    return updated


def main():
    parser = argparse.ArgumentParser(description="MDEMG Data Quality Cleanup")
    parser.add_argument("--dry-run", action="store_true", help="Preview changes without applying")
    parser.add_argument("--space-id", default="whk-wms", help="Space ID to clean up (default: whk-wms)")
    args = parser.parse_args()

    print(f"MDEMG Data Quality Cleanup {'[DRY RUN]' if args.dry_run else ''}")
    print(f"Space: {args.space_id}")
    print(f"Neo4j: {NEO4J_URI}")
    print()

    driver = GraphDatabase.driver(NEO4J_URI, auth=(NEO4J_USER, NEO4J_PASS))
    try:
        with driver.session() as session:
            print("Phase 1d: Backfill dim_semantic on existing edges...")
            backfill_dim_semantic(session, args.space_id, args.dry_run)
            print()

            print("Phase 2a: Archive duplicate nodes...")
            archive_duplicates(session, args.space_id, args.dry_run)
            print()

            print("Phase 2b: Archive stop-word nodes...")
            archive_stop_words(session, args.space_id, args.dry_run)
            print()

            print("Phase 2c: Clean orphaned edges...")
            clean_orphaned_edges(session, args.space_id, args.dry_run)
            print()

            print("Done.")
    finally:
        driver.close()


if __name__ == "__main__":
    main()
