#!/bin/bash

set -e

EMCCDEBUGFLAGS=-g4
#EMCCDEBUGFLAGS="-g0 -Oz --closure 1"

EMCCFLAGS="$EMCCDEBUGFLAGS \
	-s ENVIRONMENT=web -s STRICT=1
	-s STACK_OVERFLOW_CHECK=1 \
	-s ERROR_ON_MISSING_LIBRARIES=1 \
	-s ALLOW_MEMORY_GROWTH=1 \
	-s INVOKE_RUN=0 \
	-s MODULARIZE=1 \
	-s EXPORT_ES6=1 \
	-Werror -Wall -Wextra \
	--source-map-base http://localhost:8090/"
# -s DYNAMIC_EXECUTION=0

rm -rf build open-vcdiff/build
mkdir -p build/config

(cd build && IDL_CHECKS=all python "$EMSCRIPTEN/tools/webidl_binder.py" ../vcddec.idl vcddec_glue)

touch build/config/config.h

emcc -o build/adler32.bc $EMCCFLAGS \
	-Iopen-vcdiff/src/zlib \
	open-vcdiff/src/zlib/adler32.c

emcc -o build/libvcdcom.bc $EMCCFLAGS -std=c++17 \
	-Iopen-vcdiff/src -Ibuild/config \
	build/adler32.bc \
	open-vcdiff/src/addrcache.cc \
	open-vcdiff/src/codetable.cc \
	open-vcdiff/src/logging.cc \
	open-vcdiff/src/varint_bigendian.cc

emcc -o build/libvcddec.bc $EMCCFLAGS -std=c++17 \
	-Iopen-vcdiff/src -Iopen-vcdiff/src/zlib -Ibuild/config \
	open-vcdiff/src/decodetable.cc \
	open-vcdiff/src/headerparser.cc \
	open-vcdiff/src/vcdecoder.cc

emcc -o build/vcddec.html $EMCCFLAGS -std=c++17 \
	-Iopen-vcdiff/src -Ibuild \
	--post-js build/vcddec_glue.js \
	build/libvcd{com,dec}.bc vcddec.cc

mkdir -p open-vcdiff/build
(cd open-vcdiff/build && cmake -Dvcdiff_build_exec=off -Dvcdiff_build_tests=off ..)
(cd open-vcdiff/build && make)
