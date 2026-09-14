#!/usr/bin/env bash
# Regenerates testdata/heap_small and its expected pageinspect output from a
# local PostgreSQL server. Connection settings come from the usual PG* vars.
set -euo pipefail

cd "$(dirname "$0")/.."

db="${PGPAGE_FIXTURE_DB:-pgpage_fixture}"
out=testdata/heap_small

if [[ -z "$(psql -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname = '$db'")" ]]; then
    createdb "$db"
fi

psql -X -q -d "$db" -f scripts/make-fixture.sql

datadir="$(psql -X -d "$db" -Atc 'SHOW data_directory')"
relpath="$(psql -X -d "$db" -Atc "SELECT pg_relation_filepath('fixture_heap')")"
src="$datadir/$relpath"

mkdir -p testdata
cp "$src" "$out"
chmod 644 "$out" # PGDATA files are owner-only

# Decode the pages as they are on disk, not as they are in shared buffers:
# get_raw_page reads the buffer, whose pd_checksum is not kept up to date.
blocks="generate_series(0, pg_relation_size('fixture_heap') / 8192 - 1) AS blkno"
raw="pg_read_binary_file(pg_relation_filepath('fixture_heap'), blkno * 8192, 8192)"

psql -X -q -d "$db" -c "\copy (
    SELECT blkno, h.*
    FROM $blocks, page_header($raw) AS h
    ORDER BY blkno
) TO '$out.page_header.csv' WITH (FORMAT csv, HEADER)"

psql -X -q -d "$db" -c "\copy (
    SELECT blkno, i.lp, i.lp_off, i.lp_flags, i.lp_len, i.t_xmin, i.t_xmax,
           i.t_field3, i.t_ctid, i.t_infomask2, i.t_infomask, i.t_hoff, i.t_bits
    FROM $blocks, heap_page_items($raw) AS i
    ORDER BY blkno, i.lp
) TO '$out.items.csv' WITH (FORMAT csv, HEADER)"

# The expected output is only trustworthy if the file did not change meanwhile.
if ! cmp -s "$src" "$out"; then
    echo "make-fixture: $src changed while generating the fixture, run again" >&2
    exit 1
fi

echo "wrote $out ($(($(wc -c <"$out") / 8192)) pages) from $src"
