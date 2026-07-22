#!/usr/bin/env bash
# Enable multimodal on「轮胎异常诊断」evaluation set, switch input to MultiPart,
# and seed one sample item (text + image).
set -euo pipefail

BASE_URL="${COZE_LOOP_BASE_URL:-http://localhost:8082}"
EMAIL="${COZE_LOOP_SMOKE_EMAIL:-codex-local-smoke@example.com}"
PASSWORD="${COZE_LOOP_SMOKE_PASSWORD:-Codex123456}"
EVAL_SET_NAME="${TIRE_EVAL_SET_NAME:-轮胎异常诊断}"
MYSQL_CONTAINER="${COZE_LOOP_MYSQL_CONTAINER:-coze-loop-mysql}"
MYSQL_DB="${COZE_LOOP_MYSQL_DATABASE:-cozeloop-mysql}"
MYSQL_PASSWORD="${COZE_LOOP_MYSQL_PASSWORD:-cozeloop-mysql}"

export BASE_URL EMAIL PASSWORD EVAL_SET_NAME MYSQL_CONTAINER MYSQL_DB MYSQL_PASSWORD

python3 <<'PY'
import json, struct, time, urllib.request, zlib, subprocess

BASE_URL = __import__('os').environ['BASE_URL']
EMAIL = __import__('os').environ['EMAIL']
PASSWORD = __import__('os').environ['PASSWORD']
EVAL_SET_NAME = __import__('os').environ['EVAL_SET_NAME']
MYSQL_CONTAINER = __import__('os').environ['MYSQL_CONTAINER']
MYSQL_DB = __import__('os').environ['MYSQL_DB']
MYSQL_PASSWORD = __import__('os').environ['MYSQL_PASSWORD']


def req(url, data=None, token=None, method=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Cookie'] = f'session_key={token}'
    body = None if data is None else json.dumps(data, ensure_ascii=False).encode()
    m = method or ('POST' if data is not None else 'GET')
    r = urllib.request.Request(url, data=body, headers=headers, method=m)
    with urllib.request.urlopen(r) as resp:
        return json.load(resp)


def enable_multi_modal(dataset_id: str) -> None:
    sql = (
        "UPDATE dataset SET features = JSON_SET(COALESCE(features, JSON_OBJECT()), "
        "'$.multi_modal', true) "
        f"WHERE id = {dataset_id};"
    )
    subprocess.run(
        [
            'docker', 'exec', MYSQL_CONTAINER,
            'mysql', '-uroot', f'-p{MYSQL_PASSWORD}', MYSQL_DB,
            '-e', sql,
        ],
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )


def tiny_png() -> bytes:
    width = height = 64
    raw = b''.join(b'\x00' + bytes([40, 40, 40]) * width for _ in range(height))

    def chunk(tag, data):
        return (
            struct.pack('>I', len(data))
            + tag
            + data
            + struct.pack('>I', zlib.crc32(tag + data) & 0xFFFFFFFF)
        )

    return (
        b'\x89PNG\r\n\x1a\n'
        + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
        + chunk(b'IDAT', zlib.compress(raw))
        + chunk(b'IEND', b'')
    )


token = req(
    f'{BASE_URL}/api/foundation/v1/users/login_by_password',
    {'email': EMAIL, 'password': PASSWORD},
)['token']
space = req(
    f'{BASE_URL}/api/foundation/v1/spaces/list',
    {'page_number': 1, 'page_size': 100},
    token,
)['spaces'][0]['id']

sets = req(
    f'{BASE_URL}/api/evaluation/v1/evaluation_sets/list',
    {'workspace_id': space, 'page_number': 1, 'page_size': 200},
    token,
).get('evaluation_sets', [])
match = next((s for s in sets if s.get('name') == EVAL_SET_NAME), None)
if not match:
    raise SystemExit(f'evaluation set not found: {EVAL_SET_NAME}')
set_id = str(match['id'])
print(f'Found evaluation set: {EVAL_SET_NAME} (id={set_id})')

enable_multi_modal(set_id)
print('Enabled multi_modal in MySQL')

req(
    f'{BASE_URL}/api/evaluation/v1/evaluation_sets/{set_id}/schema',
    {
        'workspace_id': space,
        'evaluation_set_id': set_id,
        'fields': [
            {
                'key': 'input',
                'name': 'input',
                'content_type': 'MultiPart',
                'description': '轮胎图片与场景描述',
            },
            {
                'key': 'reference_output',
                'name': 'reference_output',
                'content_type': 'Text',
                'text_schema': '{"type":"string"}',
                'default_display_format': 1,
            },
        ],
    },
    token,
    method='PUT',
)
print('Updated schema: input -> MultiPart')

get = req(
    f'{BASE_URL}/api/evaluation/v1/evaluation_sets/{set_id}?workspace_id={space}',
    token=token,
)
fields = get['evaluation_set']['evaluation_set_version']['evaluation_set_schema']['field_schemas']
input_key = next(f['key'] for f in fields if f['name'] == 'input')
ref_key = next(f['key'] for f in fields if f['name'] == 'reference_output')

upload_key = f'{space}/tire-sample-{int(time.time())}.png'
sign = req(
    f'{BASE_URL}/api/foundation/v1/sign_upload_files',
    {'workspace_id': space, 'keys': [upload_key], 'business_type': 'evaluation'},
    token,
)
upload_url = BASE_URL + sign['uris'][0]
urllib.request.urlopen(urllib.request.Request(upload_url, data=tiny_png(), method='PUT'))
print(f'Uploaded sample image: {upload_key}')

listed = req(
    f'{BASE_URL}/api/evaluation/v1/evaluation_sets/{set_id}/items/list',
    {'workspace_id': space, 'evaluation_set_id': set_id, 'page_number': 1, 'page_size': 1},
    token,
)
if int(listed.get('total') or 0) > 0:
    print(f'Skip seeding: evaluation set already has {listed["total"]} item(s)')
else:
    batch = req(
        f'{BASE_URL}/api/evaluation/v1/evaluation_sets/{set_id}/items/batch_create',
        {
            'workspace_id': space,
            'evaluation_set_id': set_id,
            'skip_invalid_items': True,
            'items': [{
                'item_key': 'rf-nail-crit-sample',
                'turns': [{
                    'field_data_list': [
                        {
                            'key': input_key,
                            'name': 'input',
                            'content': {
                                'content_type': 'MultiPart',
                                'multi_part': [
                                    {
                                        'content_type': 'Text',
                                        'text': '右前轮胎面中央可见异物刺入，穿透明显（RF，1张照片）',
                                    },
                                    {
                                        'content_type': 'Image',
                                        'image': {
                                            'name': 'tire-rf-nail.png',
                                            'uri': upload_key,
                                            'storage_provider': 5,
                                        },
                                    },
                                ],
                            },
                        },
                        {
                            'key': ref_key,
                            'name': 'reference_output',
                            'content': {
                                'content_type': 'Text',
                                'text': '{"position":"RF","level":"crit","anomaly_types":["nail"],"score":35}',
                            },
                        },
                    ],
                }],
            }],
        },
        token,
    )
    if batch.get('code') != 0:
        raise SystemExit(f'batch_create failed: {batch}')
    print('Seeded 1 multipart sample item (rf-nail-crit-sample)')

print('Done. Open evaluation set in UI and refresh the page.')
PY
