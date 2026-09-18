"""Authored coding tasks for a real-agent pilot, not production bug benchmarks."""

from textwrap import dedent


def code(value):
    return dedent(value).lstrip()


CASES = [
    {
        'id': 'tenant-cache', 'module': 'cache.py',
        'goal': 'Fix tenant cache invalidation and its return contract. Follow the recorded cache findings. Preserve public signatures and generated files.',
        'findings': [
            'CACHE FINDING: storage keys are cache:{tenant}:{key}; invalidate must use that exact tenant key.',
            'CACHE FINDING: invalidate returns True only when it removed an existing key; otherwise False.',
            'CACHE FINDING: preserve other tenants, other keys, and literal spaces in identifiers.',
        ],
        'failure': 'FAIL test_invalidation: cached value remains after invalidation. The return contract is also unverified.',
        'source': code('''
            class Cache:
                def __init__(self):
                    self._data = {}

                def put(self, tenant, key, value):
                    self._data[f"cache:{tenant}:{key}"] = value

                def get(self, tenant, key):
                    return self._data.get(f"cache:{tenant}:{key}")

                def invalidate(self, tenant, key):
                    self._data.pop(f"cache:{key}", None)
        '''),
        'visible': code('''
            import unittest
            from cache import Cache

            class VisibleTests(unittest.TestCase):
                def test_invalidation(self):
                    cache = Cache()
                    cache.put("alpha", "item", 7)
                    cache.invalidate("alpha", "item")
                    self.assertIsNone(cache.get("alpha", "item"))
        '''),
        'grader': code('''
            import unittest
            from cache import Cache

            class ContractTests(unittest.TestCase):
                def test_existing_and_missing_return(self):
                    c = Cache(); c.put('a', 'x', 1)
                    self.assertIs(c.invalidate('a', 'x'), True)
                    self.assertIs(c.invalidate('a', 'x'), False)
                def test_tenant_isolation(self):
                    c = Cache(); c.put('a', 'x', 1); c.put('b', 'x', 2)
                    c.invalidate('a', 'x')
                    self.assertEqual(c.get('b', 'x'), 2)
                    self.assertIsNone(c.get('a', 'x'))
                def test_other_keys(self):
                    c = Cache(); c.put('a', 'x', 1); c.put('a', 'y', 2)
                    c.invalidate('a', 'x'); self.assertEqual(c.get('a', 'y'), 2)
                def test_none_is_existing(self):
                    c = Cache(); c.put('a', 'x', None)
                    self.assertIs(c.invalidate('a', 'x'), True)
                def test_literal_identifiers(self):
                    c = Cache(); c.put(' a ', ' x ', 1); c.put('a', 'x', 2)
                    self.assertIs(c.invalidate(' a ', ' x '), True)
                    self.assertEqual(c.get('a', 'x'), 2)
        '''),
    },
    {
        'id': 'retry-policy', 'module': 'retry.py',
        'goal': 'Fix retry behavior according to the recorded retry contract. Preserve public signatures and generated files. Do not add dependencies.',
        'findings': [
            'RETRY FINDING: retry only TimeoutError; propagate every other exception immediately.',
            'RETRY FINDING: attempts counts total calls; attempts <= 0 raises ValueError before any call.',
            'RETRY FINDING: wait 0.1 * 2**n seconds, capped at 1.0, only between attempts; n starts at zero.',
            'RETRY FINDING: after final failure, raise that exception without another sleep.',
        ],
        'failure': 'FAIL test_exhaustion: retry swallowed the last TimeoutError instead of raising it.',
        'source': code('''
            def retry(operation, attempts, sleep):
                for _ in range(attempts):
                    try:
                        return operation()
                    except Exception:
                        sleep(1)
                return None
        '''),
        'visible': code('''
            import unittest
            from retry import retry

            class VisibleTests(unittest.TestCase):
                def test_exhaustion(self):
                    def fail():
                        raise TimeoutError('timeout')
                    with self.assertRaises(TimeoutError):
                        retry(fail, 2, lambda _: None)
        '''),
        'grader': code('''
            import unittest
            from retry import retry

            class ContractTests(unittest.TestCase):
                def test_success_and_no_sleep(self):
                    waits = []; self.assertEqual(retry(lambda: 42, 3, waits.append), 42)
                    self.assertEqual(waits, [])
                def test_selective_errors(self):
                    calls = []; waits = []
                    def fail():
                        calls.append(1); raise ValueError('not retryable')
                    with self.assertRaises(ValueError): retry(fail, 4, waits.append)
                    self.assertEqual(len(calls), 1); self.assertEqual(waits, [])
                def test_backoff_and_final_exception(self):
                    calls = []; waits = []; failure = TimeoutError('last')
                    def fail():
                        calls.append(1); raise failure
                    with self.assertRaises(TimeoutError) as caught: retry(fail, 7, waits.append)
                    self.assertIs(caught.exception, failure)
                    self.assertEqual(len(calls), 7)
                    self.assertEqual(waits, [0.1, 0.2, 0.4, 0.8, 1.0, 1.0])
                def test_invalid_attempts(self):
                    calls = []; waits = []
                    for n in (0, -1):
                        with self.assertRaises(ValueError): retry(lambda: calls.append(1), n, waits.append)
                    self.assertEqual(calls, []); self.assertEqual(waits, [])
                def test_recovers_after_failure(self):
                    calls = []; waits = []
                    def eventually():
                        calls.append(1)
                        if len(calls) == 1: raise TimeoutError('temporary')
                        return 'ok'
                    self.assertEqual(retry(eventually, 2, waits.append), 'ok')
                    self.assertEqual(waits, [0.1])
        '''),
    },
    {
        'id': 'cursor-pagination', 'module': 'pagination.py',
        'goal': 'Fix cursor pagination boundaries and duplicate handling using the recorded pagination findings. Preserve public signatures and generated files.',
        'findings': [
            'PAGINATION FINDING: only a next cursor of None ends pagination; empty string and zero are valid cursors.',
            'PAGINATION FINDING: deduplicate items by id; keep the first item and its position, including id zero.',
            'PAGINATION FINDING: a repeated cursor raises ValueError before fetching that cursor again.',
            'PAGINATION FINDING: empty item pages can continue; fetch exceptions propagate unchanged.',
        ],
        'failure': 'FAIL test_empty_cursor: the second page was skipped when the next cursor was an empty string.',
        'source': code('''
            def collect_pages(fetch):
                cursor = None
                items = []
                while True:
                    page = fetch(cursor)
                    items.extend(page['items'])
                    cursor = page['next']
                    if not cursor:
                        return items
        '''),
        'visible': code('''
            import unittest
            from pagination import collect_pages

            class VisibleTests(unittest.TestCase):
                def test_empty_cursor(self):
                    pages = {None: {'items': [{'id': 1}], 'next': ''}, '': {'items': [{'id': 2}], 'next': None}}
                    self.assertEqual(len(collect_pages(pages.__getitem__)), 2)
        '''),
        'grader': code('''
            import unittest
            from pagination import collect_pages

            class ContractTests(unittest.TestCase):
                def test_empty_and_zero_cursors(self):
                    pages = {None: {'items': [{'id': 1}], 'next': ''}, '': {'items': [], 'next': 0}, 0: {'items': [{'id': 2}], 'next': None}}
                    self.assertEqual(collect_pages(pages.__getitem__), [{'id': 1}, {'id': 2}])
                def test_first_duplicate_wins(self):
                    pages = {None: {'items': [{'id': 0, 'v': 'first'}, {'id': 1}], 'next': 'b'}, 'b': {'items': [{'id': 0, 'v': 'later'}, {'id': 2}], 'next': None}}
                    self.assertEqual(collect_pages(pages.__getitem__), [{'id': 0, 'v': 'first'}, {'id': 1}, {'id': 2}])
                def test_cycle(self):
                    calls = []
                    def fetch(cursor):
                        calls.append(cursor)
                        if len(calls) > 3: raise RuntimeError('cycle was not detected')
                        return {'items': [], 'next': 'loop'}
                    with self.assertRaises(ValueError): collect_pages(fetch)
                    self.assertEqual(calls, [None, 'loop'])
                def test_empty_terminal(self):
                    self.assertEqual(collect_pages(lambda _: {'items': [], 'next': None}), [])
                def test_fetch_error(self):
                    error = OSError('offline')
                    def fail(_): raise error
                    with self.assertRaises(OSError) as caught: collect_pages(fail)
                    self.assertIs(caught.exception, error)
        '''),
    },
]


def transcript(case):
    log = [f'INFO completed historical build step {i:03d}; no failure reported.' for i in range(180)]
    log[85:85] = case['findings']
    return {
        'goal': case['goal'], 'target_tokens': 1000, 'recent_messages': 2,
        'messages': [
            {'id': 'rules', 'role': 'system', 'text': 'Keep generated_schema.py unchanged. Preserve public signatures. Use standard-library Python only.'},
            {'id': 'goal', 'role': 'user', 'text': case['goal']},
            {'id': 'investigation-call', 'role': 'assistant', 'text': '', 'tool_calls': [{'id': 'investigation', 'name': 'read_investigation_log', 'arguments': {'path': 'historical-investigation.log'}}]},
            {'id': 'investigation-result', 'role': 'tool', 'tool_call_id': 'investigation', 'text': '\n'.join(log)},
            {'id': 'failure-call', 'role': 'assistant', 'text': '', 'tool_calls': [{'id': 'failure', 'name': 'run_tests', 'arguments': {'command': 'python3 -m unittest test_visible.py'}}]},
            {'id': 'failure-result', 'role': 'tool', 'tool_call_id': 'failure', 'text': case['failure'], 'unresolved': True},
        ],
    }
