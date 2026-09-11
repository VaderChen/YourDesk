//go:build darwin && cgo

package clipboard

/*
#cgo LDFLAGS: -framework Cocoa
#cgo CFLAGS: -x objective-c
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>
static long long yd_clip_revision(void) {
 @autoreleasepool { return [NSPasteboard generalPasteboard].changeCount; }
}
static void *yd_clip_read(int *kind, int *length, int textOnly) {
 @autoreleasepool {
  NSPasteboard *board=[NSPasteboard generalPasteboard];
  // Apple 跨裝置剪貼簿不是本機新複製；重新送出會將檔案退化成檔名文字並形成回傳循環。
  if ([board.types containsObject:@"com.apple.is-remote-clipboard"]) return NULL;
  NSData *data=nil;
  if(textOnly && ([board.types containsObject:NSPasteboardTypeFileURL] || [board.types containsObject:NSPasteboardTypePNG] || [board.types containsObject:NSPasteboardTypeTIFF])){*kind=4;return NULL;}
  NSArray *urls=nil;
  if ([board.types containsObject:NSPasteboardTypeFileURL]) {
   *kind=3;
   urls=[board readObjectsForClasses:@[[NSURL class]] options:@{NSPasteboardURLReadingFileURLsOnlyKey:@YES}];
   if(!urls.count)return NULL;
  }
  if (urls.count) {
   *kind=3;
   if (urls.count>64) return NULL;
   NSMutableArray *paths=[NSMutableArray array];
   for (NSURL *url in urls) { if (url.path) [paths addObject:url.path]; }
   data=[NSJSONSerialization dataWithJSONObject:paths options:0 error:nil]; *kind=3;
  } else if ([board.types containsObject:NSPasteboardTypePNG] || [board.types containsObject:NSPasteboardTypeTIFF]) {
   *kind=2;
   data=[board dataForType:NSPasteboardTypePNG];
   if (!data) {
    NSData *tiff=[board dataForType:NSPasteboardTypeTIFF];
    if (tiff.length>134217728) return NULL;
    NSBitmapImageRep *rep=[NSBitmapImageRep imageRepWithData:tiff];
    if (!rep || rep.pixelsWide<1 || rep.pixelsHigh<1 || (long long)rep.pixelsWide*rep.pixelsHigh>33554432) return NULL;
    data=[rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
   }
   if (data.length>33554432) return NULL;
   *kind=2;
  } else {
   NSString *text=[board stringForType:NSPasteboardTypeString];
   if (!text) return NULL;
   *kind=1;
   if ([text lengthOfBytesUsingEncoding:NSUTF8StringEncoding]>1048576) return NULL;
   data=[text dataUsingEncoding:NSUTF8StringEncoding]; *kind=1;
  }
  if (!data || data.length>33554432) return NULL;
  *length=(int)data.length;
  void *result=malloc(MAX((NSUInteger)1,data.length));
  if (result && data.length) memcpy(result,data.bytes,data.length);
  return result;
 }
}
static int yd_clip_write(int kind, const void *bytes, int length) {
 @autoreleasepool {
  NSData *data=[NSData dataWithBytes:bytes length:length];
  NSPasteboard *board=[NSPasteboard generalPasteboard];
  if (kind==1) {
   NSString *text=[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
   if (!text) return 0;
   [board prepareForNewContentsWithOptions:NSPasteboardContentsCurrentHostOnly]; BOOL ok=[board setString:text forType:NSPasteboardTypeString]; [text release]; return ok;
  }
  if (kind==2) {
   NSBitmapImageRep *rep=[NSBitmapImageRep imageRepWithData:data];
   if(!rep)return 0;
   NSData *tiff=[rep TIFFRepresentation];
   [board prepareForNewContentsWithOptions:NSPasteboardContentsCurrentHostOnly];
   BOOL ok=[board setData:data forType:NSPasteboardTypePNG];
   if(tiff)[board setData:tiff forType:NSPasteboardTypeTIFF];
   return ok;
  }
  NSArray *paths=[NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
  if (![paths isKindOfClass:[NSArray class]] || !paths.count) return 0;
  NSMutableArray *urls=[NSMutableArray array];
  for (NSString *path in paths) [urls addObject:[NSURL fileURLWithPath:path]];
  [board prepareForNewContentsWithOptions:NSPasteboardContentsCurrentHostOnly]; return [board writeObjects:urls];
 }
}
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"unsafe"
)

func nativeRevision() int64            { return int64(C.yd_clip_revision()) }
func readContent() (content, error)    { return readNativeContent(false) }
func readLegacyText() (content, error) { return readNativeContent(true) }
func readNativeContent(textOnly bool) (content, error) {
	var kind, size C.int
	flag := C.int(0)
	if textOnly {
		flag = 1
	}
	ptr := C.yd_clip_read(&kind, &size, flag)
	if ptr == nil {
		if kind == 4 {
			return content{Kind: "unsupported"}, nil
		}
		if kind != 0 {
			return content{}, fmt.Errorf("剪貼簿內容過大或格式無效")
		}
		return content{}, nil
	}
	defer C.free(ptr)
	data := C.GoBytes(ptr, size)
	switch kind {
	case 1:
		return content{Kind: "text", Data: data}, nil
	case 2:
		return content{Kind: "image", Data: data}, nil
	case 3:
		var paths []string
		if err := json.Unmarshal(data, &paths); err != nil {
			return content{}, err
		}
		return content{Kind: "files", Paths: paths}, nil
	}
	return content{}, nil
}
func writeContent(value content) error {
	kind := C.int(1)
	data := value.Data
	if value.Kind == "image" {
		kind = 2
	}
	if value.Kind == "files" {
		kind = 3
		var err error
		data, err = json.Marshal(value.Paths)
		if err != nil {
			return err
		}
	}
	ptr := C.CBytes(data)
	defer C.free(unsafe.Pointer(ptr))
	if C.yd_clip_write(kind, ptr, C.int(len(data))) == 0 {
		return fmt.Errorf("系統未接受剪貼簿內容")
	}
	return nil
}
