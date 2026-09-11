//go:build windows && cgo

#include "pull_windows.h"
#include <windows.h>
#include <ole2.h>
#include <shlobj.h>
#include <shobjidl.h>
#include <atomic>
#include <mutex>
#include <string>
#include <vector>
#include <new>
#include <algorithm>
extern "C" int goYDPullRead(uintptr_t,int,int64_t,void*,int);

struct Entry { std::wstring name; uint64_t size; bool directory; };
class PullStream final: public IStream {
 std::atomic<ULONG> refs{1}; uintptr_t token; int index; uint64_t size,position=0; IUnknown *marshaler=nullptr;std::mutex mutex;std::vector<char> cache;uint64_t cacheOffset=0;
public:
 PullStream(uintptr_t t,int i,uint64_t s):token(t),index(i),size(s){CoCreateFreeThreadedMarshaler(this,&marshaler);}
 ~PullStream(){if(marshaler)marshaler->Release();}
 HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id,void **v)override{if(!v)return E_POINTER;*v=nullptr;if(id==IID_IUnknown||id==IID_ISequentialStream||id==IID_IStream){*v=static_cast<IStream*>(this);AddRef();return S_OK;}if(id==IID_IMarshal&&marshaler)return marshaler->QueryInterface(id,v);return E_NOINTERFACE;}
 ULONG STDMETHODCALLTYPE AddRef()override{return ++refs;}
 ULONG STDMETHODCALLTYPE Release()override{ULONG n=--refs;if(!n)delete this;return n;}
 HRESULT STDMETHODCALLTYPE Read(void *b,ULONG count,ULONG *read)override{
  if(read)*read=0;if(!b&&count)return STG_E_INVALIDPOINTER;std::lock_guard<std::mutex> lock(mutex);ULONG done=0;
  while(done<count&&position<size){
   if(cache.empty()||position<cacheOffset||position>=cacheOffset+cache.size()){
    int want=(int)std::min<uint64_t>(256*1024,size-position);cache.resize(want);cacheOffset=position;
    int n=goYDPullRead(token,index,(int64_t)position,cache.data(),want);if(n!=want){cache.clear();if(read)*read=done;return STG_E_READFAULT;}
   }
   ULONG n=(ULONG)std::min<uint64_t>(count-done,cacheOffset+cache.size()-position);memcpy((char*)b+done,cache.data()+(position-cacheOffset),n);position+=n;done+=n;
  }
  if(read)*read=done;return done==count?S_OK:S_FALSE;
 }
 HRESULT STDMETHODCALLTYPE Write(const void*,ULONG,ULONG*)override{return STG_E_ACCESSDENIED;}
 HRESULT STDMETHODCALLTYPE Seek(LARGE_INTEGER d,DWORD origin,ULARGE_INTEGER *out)override{std::lock_guard<std::mutex> lock(mutex);int64_t base=origin==STREAM_SEEK_SET?0:origin==STREAM_SEEK_CUR?(int64_t)position:origin==STREAM_SEEK_END?(int64_t)size:-1;if(base<0||d.QuadPart < -base||d.QuadPart>INT64_MAX-base)return STG_E_INVALIDFUNCTION;position=base+d.QuadPart;if(out)out->QuadPart=position;return S_OK;}
 HRESULT STDMETHODCALLTYPE SetSize(ULARGE_INTEGER)override{return STG_E_ACCESSDENIED;}
 HRESULT STDMETHODCALLTYPE CopyTo(IStream *other,ULARGE_INTEGER amount,ULARGE_INTEGER *read,ULARGE_INTEGER *written)override{uint64_t total=0,output=0;char b[16384];HRESULT hr=S_OK;while(total<amount.QuadPart){ULONG n=0,w=0;hr=Read(b,(ULONG)std::min<uint64_t>(sizeof b,amount.QuadPart-total),&n);total+=n;if(n){HRESULT wh=other->Write(b,n,&w);output+=w;if(FAILED(wh)||w!=n){hr=FAILED(wh)?wh:STG_E_WRITEFAULT;break;}}if(hr!=S_OK)break;}if(read)read->QuadPart=total;if(written)written->QuadPart=output;return hr;}
 HRESULT STDMETHODCALLTYPE Commit(DWORD)override{return S_OK;}
 HRESULT STDMETHODCALLTYPE Revert()override{return STG_E_REVERTED;}
 HRESULT STDMETHODCALLTYPE LockRegion(ULARGE_INTEGER,ULARGE_INTEGER,DWORD)override{return STG_E_INVALIDFUNCTION;}
 HRESULT STDMETHODCALLTYPE UnlockRegion(ULARGE_INTEGER,ULARGE_INTEGER,DWORD)override{return STG_E_INVALIDFUNCTION;}
 HRESULT STDMETHODCALLTYPE Stat(STATSTG *st,DWORD)override{if(!st)return E_POINTER;ZeroMemory(st,sizeof *st);st->type=STGTY_STREAM;st->cbSize.QuadPart=size;st->grfMode=STGM_READ;return S_OK;}
 HRESULT STDMETHODCALLTYPE Clone(IStream **out)override{if(!out)return E_POINTER;auto v=new(std::nothrow)PullStream(token,index,size);if(!v)return E_OUTOFMEMORY;{std::lock_guard<std::mutex> lock(mutex);v->position=position;}*out=v;return S_OK;}
};
class PullObject final:public IDataObject,public IDataObjectAsyncCapability {
 std::atomic<ULONG> refs{1};IUnknown *marshaler=nullptr;std::atomic<BOOL> async{TRUE};std::atomic<bool> operating{false};
 CLIPFORMAT descriptor=(CLIPFORMAT)RegisterClipboardFormatW(L"FileGroupDescriptorW"),contents=(CLIPFORMAT)RegisterClipboardFormatW(L"FileContents"),effect=(CLIPFORMAT)RegisterClipboardFormatW(L"Preferred DropEffect");
public:
 uintptr_t token;std::vector<Entry> entries;
 PullObject(uintptr_t t,int n):token(t),entries(n){}
 void initialize(){CoCreateFreeThreadedMarshaler(static_cast<IDataObject*>(this),&marshaler);}
 ~PullObject(){if(marshaler)marshaler->Release();}
 HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id,void **v)override{if(!v)return E_POINTER;*v=nullptr;if(id==IID_IUnknown||id==IID_IDataObject)*v=static_cast<IDataObject*>(this);else if(id==IID_IDataObjectAsyncCapability)*v=static_cast<IDataObjectAsyncCapability*>(this);else if(id==IID_IMarshal&&marshaler)return marshaler->QueryInterface(id,v);else return E_NOINTERFACE;AddRef();return S_OK;}
 ULONG STDMETHODCALLTYPE AddRef()override{return ++refs;}
 ULONG STDMETHODCALLTYPE Release()override{ULONG n=--refs;if(!n)delete this;return n;}
 HRESULT STDMETHODCALLTYPE QueryGetData(FORMATETC *f)override{if(!f)return E_POINTER;if(f->dwAspect!=DVASPECT_CONTENT)return DV_E_DVASPECT;if((f->cfFormat==descriptor||f->cfFormat==effect)&&(f->tymed&TYMED_HGLOBAL))return S_OK;if(f->cfFormat==contents&&(f->tymed&TYMED_ISTREAM))return S_OK;return DV_E_FORMATETC;}
 HRESULT STDMETHODCALLTYPE GetData(FORMATETC *f,STGMEDIUM *m)override{
  if(!m)return E_POINTER;ZeroMemory(m,sizeof *m);HRESULT hr=QueryGetData(f);if(FAILED(hr))return hr;
  if(f->cfFormat==contents){if(f->lindex<0||(size_t)f->lindex>=entries.size())return DV_E_LINDEX;m->pstm=new(std::nothrow)PullStream(token,f->lindex,entries[f->lindex].size);if(!m->pstm)return E_OUTOFMEMORY;m->tymed=TYMED_ISTREAM;return S_OK;}
  size_t bytes=f->cfFormat==effect?sizeof(DWORD):sizeof(UINT)+entries.size()*sizeof(FILEDESCRIPTORW);HGLOBAL mem=GlobalAlloc(GMEM_MOVEABLE|GMEM_ZEROINIT,bytes);if(!mem)return E_OUTOFMEMORY;void *p=GlobalLock(mem);if(!p){GlobalFree(mem);return E_OUTOFMEMORY;}
  if(f->cfFormat==effect)*(DWORD*)p=DROPEFFECT_COPY;else{auto group=(FILEGROUPDESCRIPTORW*)p;group->cItems=(UINT)entries.size();for(size_t i=0;i<entries.size();++i){auto &d=group->fgd[i];auto &e=entries[i];d.dwFlags=FD_ATTRIBUTES|FD_PROGRESSUI|FD_UNICODE;d.dwFileAttributes=e.directory?FILE_ATTRIBUTE_DIRECTORY:FILE_ATTRIBUTE_NORMAL;if(!e.directory){d.dwFlags|=FD_FILESIZE;d.nFileSizeHigh=(DWORD)(e.size>>32);d.nFileSizeLow=(DWORD)e.size;}wcscpy_s(d.cFileName,MAX_PATH,e.name.c_str());}}
  GlobalUnlock(mem);m->tymed=TYMED_HGLOBAL;m->hGlobal=mem;return S_OK;
 }
 HRESULT STDMETHODCALLTYPE GetDataHere(FORMATETC*,STGMEDIUM*)override{return DATA_E_FORMATETC;}
 HRESULT STDMETHODCALLTYPE GetCanonicalFormatEtc(FORMATETC*,FORMATETC *out)override{if(out)out->ptd=nullptr;return E_NOTIMPL;}
 HRESULT STDMETHODCALLTYPE SetData(FORMATETC*,STGMEDIUM *m,BOOL release)override{if(release&&m)ReleaseStgMedium(m);return S_OK;}
 HRESULT STDMETHODCALLTYPE EnumFormatEtc(DWORD direction,IEnumFORMATETC **out)override{if(direction!=DATADIR_GET)return E_NOTIMPL;FORMATETC formats[]={{descriptor,nullptr,DVASPECT_CONTENT,-1,TYMED_HGLOBAL},{contents,nullptr,DVASPECT_CONTENT,-1,TYMED_ISTREAM},{effect,nullptr,DVASPECT_CONTENT,-1,TYMED_HGLOBAL}};return SHCreateStdEnumFmtEtc(3,formats,out);}
 HRESULT STDMETHODCALLTYPE DAdvise(FORMATETC*,DWORD,IAdviseSink*,DWORD*)override{return OLE_E_ADVISENOTSUPPORTED;}
 HRESULT STDMETHODCALLTYPE DUnadvise(DWORD)override{return OLE_E_ADVISENOTSUPPORTED;}
 HRESULT STDMETHODCALLTYPE EnumDAdvise(IEnumSTATDATA**)override{return OLE_E_ADVISENOTSUPPORTED;}
 HRESULT STDMETHODCALLTYPE SetAsyncMode(BOOL value)override{async=value;return S_OK;}
 HRESULT STDMETHODCALLTYPE GetAsyncMode(BOOL *out)override{if(!out)return E_POINTER;*out=async;return S_OK;}
 HRESULT STDMETHODCALLTYPE StartOperation(IBindCtx*)override{operating=true;return S_OK;}
 HRESULT STDMETHODCALLTYPE InOperation(BOOL *out)override{if(!out)return E_POINTER;*out=operating;return S_OK;}
 HRESULT STDMETHODCALLTYPE EndOperation(HRESULT,IBindCtx*,DWORD)override{operating=false;return S_OK;}
};
struct Publish {PullObject *object;HANDLE done;HRESULT hr;};
static DWORD threadID=0;static HANDLE ready=nullptr;static HRESULT initResult=E_FAIL;static std::once_flag started;
static DWORD WINAPI clipboardThread(void*){initResult=OleInitialize(nullptr);MSG msg;PeekMessageW(&msg,nullptr,WM_USER,WM_USER,PM_NOREMOVE);SetEvent(ready);if(FAILED(initResult))return 0;while(GetMessageW(&msg,nullptr,0,0)>0){if(msg.message==WM_APP+101){auto r=(Publish*)msg.lParam;r->object->initialize();r->hr=OleSetClipboard(r->object);SetEvent(r->done);}else{TranslateMessage(&msg);DispatchMessageW(&msg);}}OleUninitialize();return 0;}
extern "C" void *yd_pull_create(uintptr_t token,int count){if(count<1||count>4096)return nullptr;try{return new PullObject(token,count);}catch(...){return nullptr;}}
extern "C" int yd_pull_entry(void *obj,int index,const char *name,int64_t size,int directory){auto o=(PullObject*)obj;if(!o||index<0||(size_t)index>=o->entries.size())return 0;int count=MultiByteToWideChar(CP_UTF8,MB_ERR_INVALID_CHARS,name,-1,nullptr,0);if(count<2||count>MAX_PATH)return 0;wchar_t wide[MAX_PATH];if(!MultiByteToWideChar(CP_UTF8,MB_ERR_INVALID_CHARS,name,-1,wide,MAX_PATH))return 0;for(auto p=wide;*p;++p)if(*p==L'/')*p=L'\\';o->entries[index]={wide,(uint64_t)size,directory!=0};return 1;}
extern "C" long yd_pull_publish(void *obj){std::call_once(started,[]{ready=CreateEventW(nullptr,TRUE,FALSE,nullptr);if(!ready)return;HANDLE t=CreateThread(nullptr,0,clipboardThread,nullptr,0,&threadID);if(!t){CloseHandle(ready);ready=nullptr;return;}CloseHandle(t);WaitForSingleObject(ready,INFINITE);CloseHandle(ready);ready=nullptr;});if(FAILED(initResult))return initResult;Publish r{(PullObject*)obj,CreateEventW(nullptr,FALSE,FALSE,nullptr),E_FAIL};if(!r.done)return E_OUTOFMEMORY;if(PostThreadMessageW(threadID,WM_APP+101,0,(LPARAM)&r))WaitForSingleObject(r.done,INFINITE);CloseHandle(r.done);return r.hr;}
extern "C" void yd_pull_discard(void *obj){if(obj)((PullObject*)obj)->Release();}
