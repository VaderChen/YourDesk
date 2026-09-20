#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
policy_jdk="${VIDEO_TEST_JDK:-${JAVA_HOME:-}}"
policy_javac=javac
policy_java=java
if [[ -n "$policy_jdk" ]]; then
  policy_javac="$policy_jdk/bin/javac"
  policy_java="$policy_jdk/bin/java"
fi
policy_tmp="$(mktemp -d "${TMPDIR:-/tmp}/yourdesk-policy-tests.XXXXXX")"
trap 'rm -f "$policy_tmp/com/yourdesk/android/FramePolicy.class" "$policy_tmp/com/yourdesk/android/FramePolicyTest.class" "$policy_tmp/com/yourdesk/android/SignalingAddress.class" "$policy_tmp/com/yourdesk/android/SignalingAddressTest.class"; rmdir "$policy_tmp/com/yourdesk/android" "$policy_tmp/com/yourdesk" "$policy_tmp/com" "$policy_tmp"' EXIT
"$policy_javac" --release 17 -d "$policy_tmp" \
  android/app/src/main/java/com/yourdesk/android/FramePolicy.java \
  android/app/src/main/java/com/yourdesk/android/SignalingAddress.java \
  android/tests/java/com/yourdesk/android/FramePolicyTest.java \
  android/tests/java/com/yourdesk/android/SignalingAddressTest.java
"$policy_java" -ea -cp "$policy_tmp" com.yourdesk.android.FramePolicyTest
"$policy_java" -ea -cp "$policy_tmp" com.yourdesk.android.SignalingAddressTest
