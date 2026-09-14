-- Builds the fixture_heap table whose main fork becomes testdata/heap_small.
-- Run through scripts/make-fixture.sh, which copies the file afterwards.

\set ON_ERROR_STOP on

CREATE EXTENSION IF NOT EXISTS pageinspect;

DROP TABLE IF EXISTS fixture_heap;

-- Autovacuum stays off so nothing rewrites the pages behind our back.
CREATE TABLE fixture_heap (
    id   int PRIMARY KEY,
    name text
) WITH (autovacuum_enabled = off);

-- About three pages of rows.
INSERT INTO fixture_heap
SELECT g, 'user ' || g
FROM generate_series(1, 500) g;

-- name is not indexed, so these are HOT updates: the new version stays on
-- the same page and is chained from the old one.
UPDATE fixture_heap SET name = name || ' v2' WHERE id % 10 = 0;

-- Deleted rows keep their tuple, with xmax set, until pruning or VACUUM.
DELETE FROM fixture_heap WHERE id % 7 = 0;

-- Reading full pages lets PostgreSQL prune them opportunistically, which
-- leaves DEAD, REDIRECT and UNUSED line pointers behind.
SELECT count(*) FROM fixture_heap;

-- Flush shared buffers so the file on disk matches what we just did.
CHECKPOINT;
