"""Offline fixture validation only; never executes a model or accesses a service."""
import argparse
import hashlib
import json
import re
import string
import sys
from collections import Counter
from pathlib import Path, PurePosixPath

ROOT = Path(__file__).resolve().parent
SCENARIOS = {'undergraduate_thesis', 'intro_paper'}
STAGES = {'prepare', 'write', 'review'}
CONDITIONS = {'supported', 'insufficient', 'existing_draft', 'different_conditions', 'injected_conflict'}
DIMENSIONS = {'任务完成度', '证据可定位与支持度', '不确定性和适用边界', '矛盾及遗漏识别', '不自动写入'}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def obj(value, fields, where):
    require(isinstance(value, dict), f'{where}: expected object')
    require(set(value) == set(fields), f'{where}: unexpected or missing fields')
    return value


def text(value, where):
    require(isinstance(value, str) and bool(value.strip()), f'{where}: expected nonempty string')
    return value


def array(value, where):
    require(isinstance(value, list) and bool(value), f'{where}: expected nonempty array')
    return value


def texts(value, where):
    items = array(value, where)
    for item in items:
        text(item, where)
    require(len(set(items)) == len(items), f'{where}: duplicate value')
    return items


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, f'duplicate JSON key: {key}')
        result[key] = value
    return result


def load(root, name):
    return json.loads((root / name).read_text(encoding='utf-8'), object_pairs_hook=unique_object)


def file_text(root, path, folder, suffix):
    text(path, 'path')
    require('\\' not in path and ':' not in path, f'invalid path: {path}')
    parts = PurePosixPath(path).parts
    require(len(parts) == 2 and parts[0] == folder and parts[1] not in ('.', '..')
            and re.fullmatch(r'[A-Za-z0-9_-]+' + re.escape(suffix), parts[1]) is not None,
            f'invalid path: {path}')
    base = root.resolve()
    target = (base / path).resolve()
    require(target.is_relative_to(base), f'path escapes root: {path}')
    data = target.read_bytes().decode('utf-8')
    return data.replace('\r\n', '\n').replace('\r', '\n')


def check_digest(content, digest, where):
    require(isinstance(digest, str) and re.fullmatch(r'[0-9a-f]{64}', digest) is not None, f'{where}: invalid sha256')
    actual = hashlib.sha256(content.encode('utf-8')).hexdigest()
    require(actual == digest, f'{where}: sha256 mismatch')


def refs(value, allowed, materials, where, allow_empty=False):
    require(isinstance(value, list) and (allow_empty or bool(value)), f'{where}: invalid references')
    seen = set()
    for ref in value:
        obj(ref, ('material', 'anchor'), where)
        mid = text(ref['material'], where)
        anchor = text(ref['anchor'], where)
        require(mid in allowed and mid in materials, f'{where}: material not in inputs: {mid}')
        require(re.fullmatch(r'[A-Z]+(?:-[A-Z]+)+', anchor) is not None, f'{where}: invalid anchor')
        require(materials[mid].count(f'[{anchor}]') == 1, f'{where}: missing or ambiguous anchor {anchor}')
        require((mid, anchor) not in seen, f'{where}: duplicate reference')
        seen.add((mid, anchor))


def validate(root):
    manifest = obj(load(root, 'manifest.json'), ('schema_version','set_version','materials','prompts','eval_cases','dev_cases','comparison'), 'manifest')
    require(type(manifest['schema_version']) is int and manifest['schema_version'] == 1, 'manifest: invalid schema version')
    text(manifest['set_version'], 'set_version')
    materials, paths = {}, set()
    for material in array(manifest['materials'], 'materials'):
        obj(material, ('id','path','license','provenance','scope','sha256'), 'material')
        mid = text(material['id'], 'material id')
        require(re.fullmatch(r'[a-z][a-z0-9_]*', mid) is not None and mid not in materials, f'duplicate or invalid material id: {mid}')
        require(material['license'] == 'CC0' and material['provenance'] == 'synthetic fixture, self-authored'
                and material['scope'] == 'entire file, including final paragraph', f'{mid}: invalid provenance or scope')
        content = file_text(root, material['path'], 'materials', '.md')
        require('Synthetic fixture / CC0' in content, f'{mid}: missing fixture label')
        check_digest(content, material['sha256'], mid)
        require(material['path'] not in paths, 'duplicate fixture path')
        paths.add(material['path'])
        materials[mid] = content
    prompts = {}
    for prompt in array(manifest['prompts'], 'prompts'):
        obj(prompt, ('id','path','sha256'), 'prompt')
        pid = text(prompt['id'], 'prompt id')
        require(pid not in prompts, f'duplicate prompt id: {pid}')
        content = file_text(root, prompt['path'], 'prompts', '.txt')
        check_digest(content, prompt['sha256'], pid)
        require(prompt['path'] not in paths, 'duplicate fixture path')
        paths.add(prompt['path'])
        prompts[pid] = content
    require(set(prompts) == {'general-v1', 'contextual-v1'}, 'unexpected prompt variants')
    for pid, content in prompts.items():
        fields = [field for _, field, _, _ in string.Formatter().parse(content) if field is not None]
        require(fields == ([] if pid == 'general-v1' else ['scenario', 'stage']), f'{pid}: invalid template fields')
    comparison = obj(manifest['comparison'], ('fixed_inputs','model','sampling','assignment','actual_execution'), 'comparison')
    for value in comparison.values():
        text(value, 'comparison')
    require(comparison['actual_execution'] == 'blocked', 'execution must remain blocked')
    ids, counts, conditions = set(), Counter(), Counter()
    for group in ('eval_cases', 'dev_cases'):
        cases = array(manifest[group], group)
        if group == 'eval_cases':
            require(len(cases) == 30, 'eval_cases: expected exactly 30')
        for case in cases:
            obj(case, ('id','scenario','stage','condition','task','materials','selection','organization_artifacts','prompt_variants','expected_boundaries','rubric_dimensions','evidence'), group)
            cid = text(case['id'], 'case id')
            pattern = r'EVAL-\d{2}' if group == 'eval_cases' else r'DEV-\d{2}'
            require(re.fullmatch(pattern, cid) is not None and cid not in ids, f'duplicate or invalid case id: {cid}')
            ids.add(cid)
            scenario, stage = case['scenario'], case['stage']
            require(scenario in SCENARIOS and stage in STAGES, f'{cid}: invalid scenario or stage')
            require(case['condition'] in (CONDITIONS if group == 'eval_cases' else {'development_only'}), f'{cid}: invalid condition')
            text(case['task'], cid)
            mids = texts(case['materials'], cid)
            require(set(mids) <= materials.keys(), f'{cid}: unknown material')
            require(case['selection'] == 'entire material files' and case['organization_artifacts'] == 'none', f'{cid}: invalid selection')
            require(case['prompt_variants'] == ['general-v1','contextual-v1'], f'{cid}: invalid prompt pair')
            texts(case['expected_boundaries'], cid)
            require(set(texts(case['rubric_dimensions'], cid)) == DIMENSIONS, f'{cid}: invalid rubric dimensions')
            refs(case['evidence'], mids, materials, cid)
            if group == 'eval_cases':
                counts[(scenario, stage)] += 1
                conditions[(scenario, stage, case['condition'])] += 1
    require(set(counts) == {(s, t) for s in SCENARIOS for t in STAGES}, 'missing scenario/stage combination')
    require(all(conditions[(s, t, c)] == 1 for s in SCENARIOS for t in STAGES for c in CONDITIONS), 'expected one of each condition per combination')
    system = obj(load(root, 'system-cases.json'), ('schema_version','cases'), 'system cases')
    require(type(system['schema_version']) is int and system['schema_version'] == 1, 'system: invalid schema version')
    categories = set()
    for case in array(system['cases'], 'system cases'):
        obj(case, ('id','category','input','expected','materials','evidence','failure','not_executed'), 'system case')
        cid = text(case['id'], 'system id')
        require(re.fullmatch(r'SYS-\d{2}', cid) is not None and cid not in ids, f'duplicate or invalid system id: {cid}')
        ids.add(cid)
        for key in ('category','input','expected','failure','not_executed'):
            text(case[key], cid)
        categories.add(case['category'])
        mids = texts(case['materials'], cid)
        require(set(mids) <= materials.keys(), f'{cid}: unknown material')
        refs(case['evidence'], mids, materials, cid)
    require({'workflow','security','safety','concurrency','recovery'} <= categories, 'missing system case category')
    retrieval = obj(load(root, 'retrieval-cases.json'), ('schema_version','corpus','top_k','conditions','queries'), 'retrieval')
    require(type(retrieval['schema_version']) is int and retrieval['schema_version'] == 1, 'retrieval: invalid schema version')
    corpus = texts(retrieval['corpus'], 'corpus')
    require(set(corpus) <= materials.keys(), 'retrieval: unknown corpus material')
    require(type(retrieval['top_k']) is int and retrieval['top_k'] == 5, 'retrieval: top_k must be 5')
    retrieval_conditions = obj(retrieval['conditions'], ('bm25','vector','fusion','degraded'), 'retrieval conditions')
    for value in retrieval_conditions.values():
        text(value, 'retrieval conditions')
    basis = Counter()
    for query in array(retrieval['queries'], 'retrieval queries'):
        obj(query, ('id','query','has_basis','relevant','expected','denominator'), 'retrieval query')
        qid = text(query['id'], 'query id')
        require(re.fullmatch(r'RET-\d{2}', qid) is not None and qid not in ids, f'duplicate or invalid query id: {qid}')
        ids.add(qid)
        text(query['query'], qid)
        text(query['expected'], qid)
        require(type(query['has_basis']) is bool, f'{qid}: has_basis must be boolean')
        supported = query['has_basis']
        refs(query['relevant'], corpus, materials, qid, allow_empty=not supported)
        require(bool(query['relevant']) == supported, f'{qid}: basis inconsistent with references')
        require(query['denominator'] == ('supported_hit_at_5' if supported else 'unsupported_behavior'), f'{qid}: invalid denominator')
        basis[supported] += 1
    require(basis[True] and basis[False], 'retrieval requires both supported and unsupported queries')
    return counts, len(manifest['dev_cases']), len(system['cases']), basis


def main():
    parser = argparse.ArgumentParser(description='Validate synthetic fixtures offline (dry-run only).')
    parser.add_argument('--root', type=Path, default=ROOT, help='fixture root (for isolated validation tests)')
    args = parser.parse_args()
    try:
        counts, dev, system, basis = validate(args.root)
    except (ValueError, OSError, TypeError, KeyError) as exc:
        print(f'INVALID: {exc}', file=sys.stderr)
        return 1
    print('DRY-RUN: validation passed; no model/API/DB calls')
    for (scenario, stage), count in sorted(counts.items()):
        print(f'  {scenario}/{stage}: {count}')
    print(f'Plan: 30 paired eval tasks / 2 prompt variants = 60 potential model calls; development tasks: {dev}, excluded')
    print(f'Independent system cases: {system}; retrieval supported: {basis[True]}, unsupported: {basis[False]} (separate denominators)')
    print('BLOCKED: actual execution entry, authorized account/model, sampling, budget and environment not implemented or confirmed; zero calls made')
    return 0


if __name__ == '__main__':
    sys.exit(main())
