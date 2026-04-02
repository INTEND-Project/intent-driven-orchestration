"""
Script for testing.
"""

import argparse
import json
import logging
import sys

from wsgiref.simple_server import make_server

logging.basicConfig(level=logging.INFO)

TEST_DATA = {
    'default/my-objective': {
        500: {
            1: {
                'p99': 120.0,
                'p95': 120.0
            },
            2: {
                'p99': 103.0,
                'p95': 103.0
            },
            3: {
                'p99': 100.0,
                'p95': 100.0
            },
            4: {
                'p99': 90.0,
                'p95': 90.0
            },
        },
        1000: {
            1: {
                'p99': 110.0,
                'p95': 110.0
            },
            2: {
                'p99': 101.0,
                'p95': 101.0
            },
            3: {
                'p99': 96.0,
                'p95': 96.0
            },
            4: {
                'p99': 85.0,
                'p95': 85.0
            },
        },
        1500: {
            1: {
                'p99': 90.0,
                'p95': 90.0
            },
            2: {
                'p99': 73.0,
                'p95': 73.0
            },
            3: {
                'p99': 70.0,
                'p95': 70.0
            },
            4: {
                'p99': 60.0,
                'p95': 60.0
            },
        }
    },
    "default/my-intent": {
        1000: {
            1: {
                'default/p99latency': 290.0,
                'default/p95latency': 250.0,
                'default/p50latency': 150.0
            },
            2: {
                'default/p99latency': 200.0,
                'default/p95latency': 200.0,
                'default/p50latency': 120.0
            },
            3: {
                'default/p99latency': 180.0,
                'default/p95latency': 180.0,
                'default/p50latency': 100.0
            },
            4: {
                'default/p99latency': 150.0,
                'default/p95latency': 150.0,
                'default/p50latency': 95.0
            },
        },
        2000: {
            1: {
                'default/p99latency': 230.0,
                'default/p95latency': 230.0,
                'default/p50latency': 130.0
            },
            2: {
                'default/p99latency': 190.0,
                'default/p95latency': 190.0,
                'default/p50latency': 105.0
            },
            3: {
                'default/p99latency': 120.0,
                'default/p95latency': 150.0,
                'default/p50latency': 90.0
            },
            4: {
                'default/p99latency': 90.0,
                'default/p95latency': 120.0,
                'default/p50latency': 85.0
            },
        },
        3000: {
            1: {
                'default/p99latency': 190.0,
                'default/p95latency': 190.0,
                'default/p50latency': 90.0
            },
            2: {
                'default/p99latency': 120.0,
                'default/p95latency': 120.0,
                'default/p50latency': 75.0
            },
            3: {
                'default/p99latency': 89.0,
                'default/p95latency': 89.0,
                'default/p50latency': 70.0
            },
            4: {
                'default/p99latency': 54.0,
                'default/p95latency': 54.0,
                'default/p50latency': 55.0
            },
        },
        4000: {
            1: {
                'default/p99latency': 150.0,
                'default/p95latency': 150.0,
                'default/p50latency': 70.0
            },
            2: {
                'default/p99latency': 95.0,
                'default/p95latency': 95.0,
                'default/p50latency': 65.0
            },
            3: {
                'default/p99latency': 70.0,
                'default/p95latency': 70.0,
                'default/p50latency': 50.0
            },
            4: {
                'default/p99latency': 46.0,
                'default/p95latency': 39.0,
                'default/p50latency': 35.0
            },
        },
    }
}


def predict_app(environ, start_response):
    """
    Predicts the effect on a latency target, or return -1.0.
    """
    try:
        body_size = int(environ.get('CONTENT_LENGTH', 0))
    except ValueError:
        body_size = 0

    request_body = environ['wsgi.input'].read(body_size)
    body = json.loads(request_body)

    name = body['name']
    target = body['target']
    cpus = body['cpus']
    cpuprofile = body['cpu_profile']

    if cpus is None or cpuprofile is None:
        sys.exit('missing values!')

    res = -1.0
    if name in TEST_DATA and \
        cpus in TEST_DATA[name] and \
        cpuprofile in TEST_DATA[name][cpus] and \
            target in TEST_DATA[name][cpus][cpuprofile]:
        res = TEST_DATA[name][cpus][cpuprofile][target]
        print(f"Predicted value for {name}, {cpus}, {cpuprofile}, {target}: {res}")


    status = '200 OK'
    headers = [('Content-type', 'application/json')]
    start_response(status, headers)

    tmp = json.dumps({'val': res})
    return [tmp.encode()]


def serve(args):
    """
    Launch a wsgi ref server.
    """
    logging.info('Listening on port: %s', int(args.port))
    with make_server('127.0.0.1', args.port, predict_app) as httpd:
        httpd.serve_forever()


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, default=8000,
                        help='Port to listen on.')
    ARGS = parser.parse_args()
    sys.stdout.write(str(serve(ARGS)))
    sys.stdout.flush()
