window.COMPACT_DEMO = {
  "model": "gpt-6-astra",
  "case_count": 3,
  "run_count": 12,
  "cases": [
    {
      "id": "tenant-cache",
      "title": "Tenant cache",
      "goal": "Fix tenant cache invalidation and its return contract. Follow the recorded cache findings. Preserve public signatures and generated files.",
      "module": "cache.py",
      "before": 2617,
      "after": 634,
      "compaction_ms": 1081.380583,
      "snapshot": "9dad7f8711f845d8b6253e93c34593e22c83320487306487c1896d542f94045b",
      "original_lines": 183,
      "original_head": [
        "INFO completed historical build step 000; no failure reported.",
        "INFO completed historical build step 001; no failure reported."
      ],
      "retained_lines": 21,
      "findings": [
        "CACHE FINDING: storage keys are cache:{tenant}:{key}; invalidate must use that exact tenant key.",
        "CACHE FINDING: invalidate returns True only when it removed an existing key; otherwise False.",
        "CACHE FINDING: preserve other tenants, other keys, and literal spaces in identifiers."
      ],
      "recall_exact": true,
      "scores": {
        "brief": 0.23,
        "drop": 0.85,
        "extract": 0.17,
        "reference": 0.72
      },
      "action": "extract",
      "patch": "--- a/cache.py\n+++ b/cache.py\n@@ -9,4 +9,5 @@\n         return self._data.get(f\"cache:{tenant}:{key}\")\n \n     def invalidate(self, tenant, key):\n-        self._data.pop(f\"cache:{key}\", None)\n+        missing = object()\n+        return self._data.pop(f\"cache:{tenant}:{key}\", missing) is not missing\n",
      "test_count": 5,
      "full_passed": 2,
      "full_total": 2,
      "compact_passed": 2,
      "compact_total": 2
    },
    {
      "id": "retry-policy",
      "title": "Retry policy",
      "goal": "Fix retry behavior according to the recorded retry contract. Preserve public signatures and generated files. Do not add dependencies.",
      "module": "retry.py",
      "before": 2648,
      "after": 657,
      "compaction_ms": 1250.928542,
      "snapshot": "28c8227aea1a89d573aec41eaa6005f24234ee2a9be1ef5f83d38bba3294fcc2",
      "original_lines": 184,
      "original_head": [
        "INFO completed historical build step 000; no failure reported.",
        "INFO completed historical build step 001; no failure reported."
      ],
      "retained_lines": 21,
      "findings": [
        "RETRY FINDING: retry only TimeoutError; propagate every other exception immediately.",
        "RETRY FINDING: attempts counts total calls; attempts <= 0 raises ValueError before any call.",
        "RETRY FINDING: wait 0.1 * 2**n seconds, capped at 1.0, only between attempts; n starts at zero.",
        "RETRY FINDING: after final failure, raise that exception without another sleep."
      ],
      "recall_exact": true,
      "scores": {
        "brief": 0.18,
        "drop": 0.85,
        "extract": 0.14,
        "reference": 0.57
      },
      "action": "extract",
      "patch": "--- a/retry.py\n+++ b/retry.py\n@@ -1,7 +1,13 @@\n def retry(operation, attempts, sleep):\n-    for _ in range(attempts):\n+    if attempts <= 0:\n+        raise ValueError('attempts must be positive')\n+\n+    delay = 0.1\n+    for attempt in range(attempts):\n         try:\n             return operation()\n-        except Exception:\n-            sleep(1)\n-    return None\n+        except TimeoutError:\n+            if attempt == attempts - 1:\n+                raise\n+            sleep(delay)\n+            delay = min(delay * 2, 1.0)\n",
      "test_count": 5,
      "full_passed": 2,
      "full_total": 2,
      "compact_passed": 2,
      "compact_total": 2
    },
    {
      "id": "cursor-pagination",
      "title": "Cursor pagination",
      "goal": "Fix cursor pagination boundaries and duplicate handling using the recorded pagination findings. Preserve public signatures and generated files.",
      "module": "pagination.py",
      "before": 2641,
      "after": 637,
      "compaction_ms": 1010.165584,
      "snapshot": "e1a9293ee12b7ee40beb9592b8ec7adfe9c457bd950db3d11c0784184839af66",
      "original_lines": 184,
      "original_head": [
        "INFO completed historical build step 000; no failure reported.",
        "INFO completed historical build step 001; no failure reported."
      ],
      "retained_lines": 21,
      "findings": [
        "PAGINATION FINDING: only a next cursor of None ends pagination; empty string and zero are valid cursors.",
        "PAGINATION FINDING: deduplicate items by id; keep the first item and its position, including id zero.",
        "PAGINATION FINDING: a repeated cursor raises ValueError before fetching that cursor again.",
        "PAGINATION FINDING: empty item pages can continue; fetch exceptions propagate unchanged."
      ],
      "recall_exact": true,
      "scores": {
        "brief": 0.23,
        "drop": 0.83,
        "extract": 0.16,
        "reference": 0.74
      },
      "action": "extract",
      "patch": "--- a/pagination.py\n+++ b/pagination.py\n@@ -1,9 +1,18 @@\n def collect_pages(fetch):\n     cursor = None\n     items = []\n+    seen_cursors = set()\n+    seen_ids = set()\n     while True:\n+        if cursor in seen_cursors:\n+            raise ValueError('Repeated pagination cursor')\n+        seen_cursors.add(cursor)\n         page = fetch(cursor)\n-        items.extend(page['items'])\n+        for item in page['items']:\n+            item_id = item['id']\n+            if item_id not in seen_ids:\n+                seen_ids.add(item_id)\n+                items.append(item)\n         cursor = page['next']\n-        if not cursor:\n+        if cursor is None:\n             return items\n",
      "test_count": 5,
      "full_passed": 2,
      "full_total": 2,
      "compact_passed": 2,
      "compact_total": 2
    }
  ]
};
