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
	unsigned char *data;
	size_t len;
	OSStatus status;
} yd_jpeg_result;

static void yd_jpeg_callback(
	void *ref,
	void *sourceFrameRefCon,
	OSStatus status,
	VTEncodeInfoFlags flags,
	CMSampleBufferRef sampleBuffer
) {
	yd_jpeg_result *result = (yd_jpeg_result *)ref;
	result->status = status;
	if (status != noErr || sampleBuffer == NULL) return;

	CMBlockBufferRef block = CMSampleBufferGetDataBuffer(sampleBuffer);
	if (block == NULL) {
		result->status = kVTVideoEncoderMalfunctionErr;
		return;
	}
	size_t length = CMBlockBufferGetDataLength(block);
	if (length == 0) {
		result->status = kVTVideoEncoderMalfunctionErr;
		return;
	}
	unsigned char *data = (unsigned char *)malloc(length);
	if (data == NULL) {
		result->status = kVTAllocationFailedErr;
		return;
	}
	OSStatus copyStatus = CMBlockBufferCopyDataBytes(block, 0, length, data);
	if (copyStatus != kCMBlockBufferNoErr) {
		free(data);
		result->status = copyStatus;
		return;
	}
	result->data = data;
	result->len = length;
}

static CFDictionaryRef yd_hardware_encoder_spec(void) {
	const void *keys[] = {
		kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder,
	};
	const void *values[] = { kCFBooleanTrue };
	return CFDictionaryCreate(
		NULL, keys, values, 1,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
}

static int yd_using_hardware_encoder(VTCompressionSessionRef session) {
	CFTypeRef value = NULL;
	OSStatus status = VTSessionCopyProperty(
		session,
		kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder,
		NULL,
		&value
	);
	int hardware = status == noErr && value != NULL &&
		CFGetTypeID(value) == CFBooleanGetTypeID() &&
		CFBooleanGetValue((CFBooleanRef)value);
	if (value != NULL) CFRelease(value);
	return hardware;
}

static int yd_create_hardware_jpeg_session(
	int width,
	int height,
	yd_jpeg_result *result,
	VTCompressionSessionRef *sessionOut
) {
	if (width <= 0 || height <= 0 || result == NULL || sessionOut == NULL) return 0;
	CFDictionaryRef spec = yd_hardware_encoder_spec();
	if (spec == NULL) return 0;
	*sessionOut = NULL;
	OSStatus status = VTCompressionSessionCreate(
		NULL,
		width,
		height,
		kCMVideoCodecType_JPEG,
		spec,
		NULL,
		NULL,
		yd_jpeg_callback,
		result,
		sessionOut
	);
	CFRelease(spec);
	if (status != noErr || *sessionOut == NULL) return 0;
	if (!yd_using_hardware_encoder(*sessionOut)) {
		VTCompressionSessionInvalidate(*sessionOut);
		CFRelease(*sessionOut);
		*sessionOut = NULL;
		return 0;
	}
	return 1;
}

static int yd_hardware_jpeg_encode(
	const unsigned char *rgba,
	size_t rgbaStride,
	int width,
	int height,
	float quality,
	unsigned char **outputData,
	size_t *outputLength
) {
	if (rgba == NULL || outputData == NULL || outputLength == NULL ||
		width <= 0 || height <= 0 || rgbaStride < (size_t)width * 4) return 0;
	*outputData = NULL;
	*outputLength = 0;

	CVPixelBufferRef pixelBuffer = NULL;
	CVReturn pixelStatus = CVPixelBufferCreate(
		NULL,
		width,
		height,
		kCVPixelFormatType_32BGRA,
		NULL,
		&pixelBuffer
	);
	if (pixelStatus != kCVReturnSuccess || pixelBuffer == NULL) return 0;
	pixelStatus = CVPixelBufferLockBaseAddress(pixelBuffer, 0);
	if (pixelStatus != kCVReturnSuccess) {
		CFRelease(pixelBuffer);
		return 0;
	}
	unsigned char *destination = (unsigned char *)CVPixelBufferGetBaseAddress(pixelBuffer);
	size_t destinationStride = CVPixelBufferGetBytesPerRow(pixelBuffer);
	if (destination == NULL || destinationStride < (size_t)width * 4) {
		CVPixelBufferUnlockBaseAddress(pixelBuffer, 0);
		CFRelease(pixelBuffer);
		return 0;
	}
	for (int y = 0; y < height; ++y) {
		const unsigned char *sourceRow = rgba + ((size_t)y * rgbaStride);
		unsigned char *destinationRow = destination + ((size_t)y * destinationStride);
		for (int x = 0; x < width; ++x) {
			const unsigned char *sourcePixel = sourceRow + ((size_t)x * 4);
			unsigned char *destinationPixel = destinationRow + ((size_t)x * 4);
			// Go image.RGBA is RGBA; CoreVideo's buffer is BGRA.
			destinationPixel[0] = sourcePixel[2];
			destinationPixel[1] = sourcePixel[1];
			destinationPixel[2] = sourcePixel[0];
			destinationPixel[3] = sourcePixel[3];
		}
	}
	CVPixelBufferUnlockBaseAddress(pixelBuffer, 0);

	yd_jpeg_result result = { .data = NULL, .len = 0, .status = noErr };
	VTCompressionSessionRef session = NULL;
	if (!yd_create_hardware_jpeg_session(width, height, &result, &session)) {
		CFRelease(pixelBuffer);
		return 0;
	}

	if (quality < 0.01f) quality = 0.01f;
	if (quality > 1.0f) quality = 1.0f;
	CFNumberRef qualityNumber = CFNumberCreate(NULL, kCFNumberFloat32Type, &quality);
	OSStatus status = qualityNumber == NULL ? kVTAllocationFailedErr :
		VTSessionSetProperty(session, kVTCompressionPropertyKey_Quality, qualityNumber);
	if (qualityNumber != NULL) CFRelease(qualityNumber);
	if (status == noErr) status = VTCompressionSessionPrepareToEncodeFrames(session);
	if (status == noErr) {
		status = VTCompressionSessionEncodeFrame(
			session,
			pixelBuffer,
			CMTimeMake(0, 1),
			kCMTimeInvalid,
			NULL,
			NULL,
			NULL
		);
	}
	if (status == noErr) status = VTCompressionSessionCompleteFrames(session, kCMTimeInvalid);

	int validJPEG = result.len >= 4 && result.data != NULL &&
		result.data[0] == 0xFF && result.data[1] == 0xD8 &&
		result.data[result.len - 2] == 0xFF && result.data[result.len - 1] == 0xD9;
	int success = status == noErr && result.status == noErr && validJPEG;
	if (success) {
		*outputData = result.data;
		*outputLength = result.len;
	} else if (result.data != NULL) {
		free(result.data);
	}

	VTCompressionSessionInvalidate(session);
	CFRelease(session);
	CFRelease(pixelBuffer);
	return success;
}

// Session allocation alone is insufficient. Exercise the exact encode path
// and require the documented hardware property before advertising capability.
static int yd_probe_hardware_jpeg_encoder(void) {
	unsigned char rgba[16 * 16 * 4];
	for (int y = 0; y < 16; ++y) {
		for (int x = 0; x < 16; ++x) {
			size_t offset = ((size_t)y * 16 + (size_t)x) * 4;
			rgba[offset + 0] = (unsigned char)(x * 16);
			rgba[offset + 1] = (unsigned char)(y * 16);
			rgba[offset + 2] = (unsigned char)((x + y) * 8);
			rgba[offset + 3] = 255;
		}
	}
	unsigned char *data = NULL;
	size_t length = 0;
	int success = yd_hardware_jpeg_encode(rgba, 16 * 4, 16, 16, 0.70f, &data, &length);
	if (data != NULL) free(data);
	return success && length > 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"unsafe"
)

type darwinJPEGEncoder struct{}

func newHardwareJPEGEncoder() (JPEGEncoder, error) {
	if C.yd_probe_hardware_jpeg_encoder() == 0 {
		return nil, ErrHardwareJPEGUnavailable
	}
	return darwinJPEGEncoder{}, nil
}

func (darwinJPEGEncoder) Encode(img image.Image, quality int) ([]byte, error) {
	if img == nil {
		return nil, errors.New("VideoToolbox JPEG: nil image")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("VideoToolbox JPEG: image dimensions must be positive")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	if len(rgba.Pix) == 0 || rgba.Stride < width*4 {
		return nil, errors.New("VideoToolbox JPEG: invalid RGBA buffer")
	}
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}

	var output *C.uchar
	var outputLength C.size_t
	success := C.yd_hardware_jpeg_encode(
		(*C.uchar)(unsafe.Pointer(&rgba.Pix[0])),
		C.size_t(rgba.Stride),
		C.int(width),
		C.int(height),
		C.float(float32(quality)/100.0),
		&output,
		&outputLength,
	)
	if success == 0 || output == nil || outputLength == 0 {
		return nil, errors.New("VideoToolbox JPEG encode failed")
	}
	defer C.free(unsafe.Pointer(output))
	// C.GoBytes accepts a C.int length.
	if uint64(outputLength) > uint64(^uint32(0)>>1) {
		return nil, fmt.Errorf("VideoToolbox JPEG result too large: %d bytes", uint64(outputLength))
	}
	return C.GoBytes(unsafe.Pointer(output), C.int(outputLength)), nil
}

func (darwinJPEGEncoder) Backend() string { return "VideoToolbox JPEG" }
func (darwinJPEGEncoder) Hardware() bool  { return true }
func (darwinJPEGEncoder) Close() error    { return nil }

// Decode remains fail-closed until a CMSampleBuffer-to-CVPixelBuffer wrapper
// is implemented and verified; capability probing alone is not a decoder.
func newHardwareJPEGDecoder() (JPEGDecoder, error) { return nil, ErrHardwareJPEGUnavailable }
