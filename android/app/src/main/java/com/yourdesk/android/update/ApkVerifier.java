package com.yourdesk.android.update;

import android.content.Context;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;
import android.os.Build;
import java.io.File;
import java.io.IOException;
import java.util.Arrays;

/** 安裝前獨立核對套件 ID、版號與現有安裝的簽章；不清除站台或密碼。 */
public final class ApkVerifier {
  private ApkVerifier() { }

  @SuppressWarnings("deprecation")
  public static PackageInfo installed(Context context) throws PackageManager.NameNotFoundException {
    return context.getPackageManager().getPackageInfo(context.getPackageName(),
        Build.VERSION.SDK_INT >= 28 ? PackageManager.GET_SIGNING_CERTIFICATES : PackageManager.GET_SIGNATURES);
  }

  @SuppressWarnings("deprecation")
  public static long code(PackageInfo info) {
    return Build.VERSION.SDK_INT >= 28 ? info.getLongVersionCode() : info.versionCode;
  }

  @SuppressWarnings("deprecation")
  public static void verify(Context context, File apk, ReleaseUpdate update) throws IOException {
    try {
      PackageInfo current = installed(context);
      PackageInfo candidate = context.getPackageManager().getPackageArchiveInfo(apk.getAbsolutePath(),
          Build.VERSION.SDK_INT >= 28 ? PackageManager.GET_SIGNING_CERTIFICATES : PackageManager.GET_SIGNATURES);
      if (candidate == null || !context.getPackageName().equals(candidate.packageName)
          || code(candidate) != update.versionCode || code(candidate) <= code(current)
          || !update.version.equals(candidate.versionName) || candidate.applicationInfo == null
          || candidate.applicationInfo.minSdkVersion > Build.VERSION.SDK_INT
          || (candidate.applicationInfo.flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) throw invalid();
      Signature[] incoming, existing, history;
      if (Build.VERSION.SDK_INT >= 28) {
        if (candidate.signingInfo == null || current.signingInfo == null
            || candidate.signingInfo.hasMultipleSigners() || current.signingInfo.hasMultipleSigners()) throw invalid();
        incoming = candidate.signingInfo.getApkContentsSigners();
        existing = current.signingInfo.getApkContentsSigners();
        // 只接受相同金鑰，或 APK 已證明從目前金鑰向前輪替的 lineage。
        history = candidate.signingInfo.getSigningCertificateHistory();
      } else {
        incoming = candidate.signatures; existing = current.signatures; history = incoming;
      }
      if (incoming == null || existing == null || history == null || incoming.length != 1 || existing.length != 1
          || !UpdateFiles.sha256(incoming[0].toByteArray()).equals(update.signerHash)) throw invalid();
      for (Signature signer : history) if (Arrays.equals(signer.toByteArray(), existing[0].toByteArray())) return;
      throw invalid();
    } catch (PackageManager.NameNotFoundException | RuntimeException error) {
      throw new IOException("Cannot verify Android update signing identity", error);
    }
  }

  private static IOException invalid() { return new IOException("Android APK identity, version or signature mismatch"); }
}
