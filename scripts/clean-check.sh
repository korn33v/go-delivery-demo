#!/bin/sh
set -eu
# Удаляются только volumes проекта go-delivery-demo. Остальные проекты не затрагиваются.
make down
make reset
make up
make demo
make observe
