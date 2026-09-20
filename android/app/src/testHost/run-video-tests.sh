#!/usr/bin/env bash
set -euo pipefail

# Run with JDK 17+. VIDEO_TEST_JDK may point to a JDK without changing system installations.
cd -- "$(dirname -- "$0")/../../../.."
video_javac=javac
video_java=java
if [[ -n "${VIDEO_TEST_JDK:-}" ]]; then
  video_javac="$VIDEO_TEST_JDK/bin/javac"
  video_java="$VIDEO_TEST_JDK/bin/java"
fi
video_test_build=$(mktemp -d "${TMPDIR:-/tmp}/yourdesk-video-host.XXXXXX")
trap 'rm -rf -- "${video_test_build:?}"' EXIT
video_test_sources=()
while IFS= read -r -d '' video_source; do
  video_test_sources+=("$video_source")
done < <(find android/app/src/testHost/java -type f -name '*.java' ! -name '._*' -print0)
"$video_javac" --release 17 -Xlint:all -d "$video_test_build" \
  "${video_test_sources[@]}" \
  android/app/src/main/java/com/yourdesk/android/video/EncodedVideoFrame.java \
  android/app/src/main/java/com/yourdesk/android/video/DecoderSupport.java \
  android/app/src/main/java/com/yourdesk/android/video/MediaCodecVideoDecoder.java
"$video_java" -Xmx128m -cp "$video_test_build" com.yourdesk.android.video.VideoDecoderHostTest \
  android/app/src/androidTest/assets/fullscreen-h264-1080.bin
