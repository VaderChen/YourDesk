//go:build darwin && cgo

package video

/*
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation
#include <CoreMedia/CoreMedia.h>
#include <CoreVideo/CoreVideo.h>
#include <VideoToolbox/VideoToolbox.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

typedef struct {
	OSStatus status;
	size_t length;
	unsigned char *bytes;
} yd_probe_result;

static void yd_probe_callback(void *ref, void *sourceFrameRefCon, OSStatus status, VTEncodeInfoFlags flags, CMSampleBufferRef sampleBuffer) {
	yd_probe_result *result = (yd_probe_result *)ref;
	result->status = status;
	if (status != noErr || sampleBuffer == NULL) return;
	CMBlockBufferRef block = CMSampleBufferGetDataBuffer(sampleBuffer);
	if (block == NULL) return;
	size_t length = CMBlockBufferGetDataLength(block);
	if (length == 0) return;
	unsigned char *bytes = (unsigned char *)malloc(length);
	if (bytes == NULL) return;
	if (CMBlockBufferCopyDataBytes(block, 0, length, bytes) != kCMBlockBufferNoErr) {
		free(bytes);
		return;
	}
	result->bytes = bytes;
	result->length = length;
}

static int yd_probe_encoder_hw_property(VTCompressionSessionRef session) {
	CFTypeRef value = NULL;
	OSStatus status = VTSessionCopyProperty(session, kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder, NULL, &value);
	int result = status == noErr && value != NULL && CFGetTypeID(value) == CFBooleanGetTypeID() && CFBooleanGetValue((CFBooleanRef)value);
	if (value != NULL) CFRelease(value);
	return result;
}

static int yd_probe_jpeg_encoder(void) {
	const int width = 16;
	const int height = 16;
	unsigned char rgba[16 * 16 * 4];
	for (int i = 0; i < width * height; ++i) {
		rgba[i * 4 + 0] = (unsigned char)(i & 0xff);
		rgba[i * 4 + 1] = (unsigned char)((i * 3) & 0xff);
		rgba[i * 4 + 2] = (unsigned char)((i * 7) & 0xff);
		rgba[i * 4 + 3] = 255;
	}
	CVPixelBufferRef pixelBuffer = NULL;
	if (CVPixelBufferCreate(NULL, width, height, kCVPixelFormatType_32BGRA, NULL, &pixelBuffer) != kCVReturnSuccess || pixelBuffer == NULL) return 0;
	if (CVPixelBufferLockBaseAddress(pixelBuffer, 0) != kCVReturnSuccess) {
		CFRelease(pixelBuffer);
		return 0;
	}
	unsigned char *destination = (unsigned char *)CVPixelBufferGetBaseAddress(pixelBuffer);
	size_t destinationStride = CVPixelBufferGetBytesPerRow(pixelBuffer);
	for (int y = 0; y < height; ++y) {
		for (int x = 0; x < width; ++x) {
			const unsigned char *source = rgba + ((size_t)y * width + x) * 4;
			unsigned char *target = destination + (size_t)y * destinationStride + (size_t)x * 4;
			target[0] = source[2];
			target[1] = source[1];
			target[2] = source[0];
			target[3] = source[3];
		}
	}
	CVPixelBufferUnlockBaseAddress(pixelBuffer, 0);

	const void *keys[] = { kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder };
	const void *values[] = { kCFBooleanTrue };
	CFDictionaryRef spec = CFDictionaryCreate(NULL, keys, values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	VTCompressionSessionRef session = NULL;
	yd_probe_result result = { .status = noErr, .length = 0, .bytes = NULL };
	OSStatus status = spec == NULL ? kVTAllocationFailedErr : VTCompressionSessionCreate(NULL, width, height, kCMVideoCodecType_JPEG, spec, NULL, NULL, yd_probe_callback, &result, &session);
	if (spec != NULL) CFRelease(spec);
	if (status != noErr || session == NULL || !yd_probe_encoder_hw_property(session)) {
		if (session != NULL) {
			VTCompressionSessionInvalidate(session);
			CFRelease(session);
		}
		CFRelease(pixelBuffer);
		return 0;
	}
	status = VTCompressionSessionPrepareToEncodeFrames(session);
	if (status == noErr) status = VTCompressionSessionEncodeFrame(session, pixelBuffer, CMTimeMake(0, 1), kCMTimeInvalid, NULL, NULL, NULL);
	if (status == noErr) status = VTCompressionSessionCompleteFrames(session, kCMTimeInvalid);
	int success = status == noErr && result.status == noErr && result.bytes != NULL && result.length >= 4 && result.bytes[0] == 0xff && result.bytes[1] == 0xd8 && result.bytes[result.length - 2] == 0xff && result.bytes[result.length - 1] == 0xd9;
	if (result.bytes != NULL) free(result.bytes);
	VTCompressionSessionInvalidate(session);
	CFRelease(session);
	CFRelease(pixelBuffer);
	return success;
}

static int yd_probe_jpeg_decoder(void) {
	return VTIsHardwareDecodeSupported(kCMVideoCodecType_JPEG) ? 1 : 0;
}
*/
import "C"

func init() {
	RegisterHardwareJPEGProbe(func() JPEGCapabilities {
		encode := C.yd_probe_jpeg_encoder() == 1
		decode := C.yd_probe_jpeg_decoder() == 1
		detail := "VideoToolbox JPEG hardware encode/decode runtime probe"
		if !encode && !decode {
			detail = "VideoToolbox 沒有通過實測的硬體 JPEG encode/decode path"
		} else if !encode {
			detail = "VideoToolbox JPEG 僅硬體解碼探測通過"
		} else if !decode {
			detail = "VideoToolbox JPEG 僅硬體編碼探測通過"
		}
		return JPEGCapabilities{Encode: encode, Decode: decode, Backend: "VideoToolbox JPEG", Detail: detail, Probed: true}
	})
}
