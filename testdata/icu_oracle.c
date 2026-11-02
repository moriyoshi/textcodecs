/* Produce fixed-size records for independent differential testing with ICU. */
#include <stdint.h>
#include <stdio.h>
#include <unicode/ucnv.h>
#include <unicode/ucnv_err.h>
#include <unicode/ustring.h>
#include <unicode/utf16.h>
#include <unicode/uversion.h>

static void record(int ok, const char *buf, int len) {
 unsigned char out[9] = {0};
 if (ok && len >= 0 && len <= 8) { out[0] = (unsigned char)(len + 1); for (int i=0;i<len;i++) out[i+1] = (unsigned char)buf[i]; }
 if (fwrite(out, sizeof(out), 1, stdout) != 1) { perror("fwrite"); }
}

int main(int argc, char **argv) {
 UErrorCode e=U_ZERO_ERROR;
 UVersionInfo v; u_getVersion(v);
 if (v[0] != 74 || v[1] != 2) { fprintf(stderr,"ICU 74.2 required\n"); return 1; }
 if (argc != 1 && argc != 3) return 2;
 UConverter *c=argc == 1 ? ucnv_open("ibm-943_P15A-2003",&e) : ucnv_openPackage(argv[1],argv[2],&e);
 if (U_FAILURE(e)) {fprintf(stderr,"open: %s\n",u_errorName(e));return 1;}
 ucnv_setToUCallBack(c, UCNV_TO_U_CALLBACK_STOP, NULL, NULL, NULL, &e);
 ucnv_setFromUCallBack(c, UCNV_FROM_U_CALLBACK_STOP, NULL, NULL, NULL, &e);
 if (U_FAILURE(e)) return 1;
 for (int len=1;len<=2;len++) {
  int max = len == 1 ? 256 : 65536;
  for (int i=0;i<max;i++) {
   char src[2]={(char)(len == 1 ? i : i>>8),(char)i}, utf8[8]={0};
   UChar u[4]; int32_t n8=0; e=U_ZERO_ERROR; ucnv_reset(c);
   int n=ucnv_toUChars(c,u,4,src,len,&e);
   if (U_SUCCESS(e)) u_strToUTF8(utf8,8,&n8,u,n,&e);
   record(U_SUCCESS(e),utf8,n8);
  }
 }
 int maxRune = argc == 1 ? 0x10ffff : 0xffff;
 for (int r=0;r<=maxRune;r++) {
  char out[8]={0}; UChar in[2]; int32_t n=0;
  if (r>=0xd800 && r<=0xdfff) { record(0,out,0); continue; }
  U16_APPEND_UNSAFE(in,n,r); e=U_ZERO_ERROR; ucnv_reset(c);
  int count=ucnv_fromUChars(c,out,8,in,n,&e);
  record(U_SUCCESS(e),out,count);
 }
 ucnv_close(c);
 return ferror(stdout) ? 1 : 0;
}
