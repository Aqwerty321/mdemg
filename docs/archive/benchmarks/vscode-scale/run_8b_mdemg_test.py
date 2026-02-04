#!/usr/bin/env python3
"""
V4 Evidence-Locked Benchmark: 8B Model + MDEMG
Tests if a small model (Qwen 2.5 Coder 7B) can match large model performance
when augmented with MDEMG retrieval.
"""

import json
import requests
import time

OLLAMA_URL = "http://localhost:11434/api/generate"
MDEMG_URL = "http://localhost:8090/v1/memory/retrieve"
MODEL = "qwen2.5-coder:7b"

QUESTIONS = [
    {"id": "ev_001", "query": "EDITOR_FONT_DEFAULTS fontSize editorOptions default font size platform macOS Windows"},
    {"id": "ev_002", "query": "DEFAULT_FLUSH_INTERVAL storage flush interval milliseconds"},
    {"id": "ev_003", "query": "extension activation timeout milliseconds Promise.race"},
    {"id": "ev_004", "query": "SidebarPart minimumWidth activitybar ACTION_HEIGHT pixels"},
    {"id": "ev_005", "query": "CodeLens debounce delay setTimeout milliseconds"},
    {"id": "ev_007", "query": "hover delay EditorOptions HoverController milliseconds default"},
    {"id": "ev_008", "query": "autoSaveDelay DEFAULT_AUTO_SAVE_DELAY minimum milliseconds"},
    {"id": "ev_011", "query": "quickSuggestionsDelay suggest widget milliseconds default"},
    {"id": "ev_014", "query": "search maxResults DEFAULT_MAX_SEARCH_RESULTS ripgrep limit"},
]

def query_mdemg(query: str) -> list:
    """Query MDEMG for relevant files."""
    try:
        response = requests.post(
            MDEMG_URL,
            json={"space_id": "vscode-scale", "query_text": query, "candidate_k": 100, "top_k": 5},
            timeout=30
        )
        data = response.json().get("results", [])
        return [{"path": r.get("path", ""), "score": r.get("score", 0)} for r in data[:3]]
    except Exception as e:
        return [{"error": str(e)}]

def query_8b_model(question: str, context: list) -> str:
    """Query the 8B model with MDEMG context."""
    context_str = "\n".join([f"- {c['path']} (score: {c.get('score', 'N/A'):.2f})" for c in context if 'path' in c])

    prompt = f"""You are answering a VS Code codebase question. You have access to these relevant files from MDEMG retrieval:

{context_str}

Based on your knowledge of VS Code internals and the file paths above, answer this question:
{question}

Provide your answer as JSON with: file_path, symbol_name, value (if known), confidence (high/medium/low).
Keep your response brief and focused on the evidence."""

    try:
        response = requests.post(
            OLLAMA_URL,
            json={
                "model": MODEL,
                "prompt": prompt,
                "stream": False,
                "options": {"temperature": 0.1, "num_predict": 300}
            },
            timeout=60
        )
        return response.json().get("response", "No response")
    except Exception as e:
        return f"Error: {e}"

def run_benchmark():
    """Run the 8B + MDEMG benchmark."""
    results = []

    print(f"Running V4 Evidence-Locked Benchmark with {MODEL} + MDEMG")
    print("=" * 60)

    for q in QUESTIONS:
        print(f"\n[{q['id']}] Querying MDEMG...")

        # Step 1: Query MDEMG
        mdemg_results = query_mdemg(q['query'])
        print(f"  MDEMG returned: {len(mdemg_results)} results")
        for r in mdemg_results:
            if 'path' in r:
                print(f"    - {r['path'][:60]}... ({r['score']:.2f})")

        # Step 2: Query 8B model with context
        print(f"  Querying {MODEL}...")
        start = time.time()
        answer = query_8b_model(q['query'], mdemg_results)
        elapsed = time.time() - start

        print(f"  Response ({elapsed:.1f}s): {answer[:200]}...")

        results.append({
            "id": q['id'],
            "mdemg_results": mdemg_results,
            "model_response": answer,
            "latency_s": elapsed
        })

    print("\n" + "=" * 60)
    print("BENCHMARK COMPLETE")
    print(f"Total questions: {len(results)}")

    # Save results
    with open("/Users/reh3376/mdemg/docs/tests/vscode-scale/8b_mdemg_results.json", "w") as f:
        json.dump(results, f, indent=2)
    print("Results saved to 8b_mdemg_results.json")

    return results

if __name__ == "__main__":
    run_benchmark()
