# Copyright (c) 2022 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

FROM golang:1.22 AS builder

WORKDIR /plugins

COPY . ./

RUN make prepare-build build-plugins \
    && go run github.com/google/go-licenses@v1.6.0 save "./..." --save_path licenses \
    && hack/additional-licenses.sh

FROM alpine:3.20

RUN adduser -D nonroot
RUN apk add --upgrade --no-cache openssl=~3.3 && apk add --no-cache python3=~3.12 py3-matplotlib=~3.7 \
    py3-pip=~24.0 py3-scikit-learn=~1.3
RUN pip install --break-system-packages --no-cache-dir pymongo~=4.6

WORKDIR /plugins

COPY --from=builder /plugins/bin/plugins/cpu_profile /plugins/bin/plugins/cpu_profile
COPY --from=builder /plugins/licenses ./licenses
COPY pkg/planner/actuators/profiling/analytics/analyze.py /plugins/pkg/planner/actuators/profiling/analyze.py
COPY pkg/planner/actuators/profiling/analytics/predict.py /plugins/pkg/planner/actuators/profiling/predict.py
USER nonroot:nonroot
EXPOSE 33334

ENTRYPOINT ["/plugins/bin/plugins/cpu_profile"]

