/* Test-only exhaustive oracle for an ICU-compiled single-byte UCM. */
#include <stdint.h>
#include <stdio.h>
#include <unicode/ucnv.h>
#include <unicode/ucnv_err.h>

static void write32(uint32_t n) {
 unsigned char b[4] = {(unsigned char)n,(unsigned char)(n>>8),(unsigned char)(n>>16),(unsigned char)(n>>24)};
 if (fwrite(b,4,1,stdout)!=1) { perror("write"); }
}
int main(int argc,char **argv) {
 if (argc!=3) return 2;
 UErrorCode e=U_ZERO_ERROR; UConverter *c=ucnv_openPackage(argv[1],argv[2],&e);
 if (U_FAILURE(e)) {fprintf(stderr,"open: %s\n",u_errorName(e));return 1;}
 ucnv_setToUCallBack(c,UCNV_TO_U_CALLBACK_STOP,NULL,NULL,NULL,&e);
 ucnv_setFromUCallBack(c,UCNV_FROM_U_CALLBACK_STOP,NULL,NULL,NULL,&e);
 if (U_FAILURE(e)) return 1;
 for (int b=0;b<256;b++) {
  char src=(char)b;UChar dst[4];e=U_ZERO_ERROR;ucnv_reset(c);
  int n=ucnv_toUChars(c,dst,4,&src,1,&e);
  write32(U_SUCCESS(e) && n==1 ? (uint32_t)dst[0]+1 : 0);
 }
 for (int r=0;r<65536;r++) {
  if (r>=0xd800 && r<=0xdfff) {write32(0);continue;}
  UChar src=(UChar)r;char dst[8];e=U_ZERO_ERROR;ucnv_reset(c);
  int n=ucnv_fromUChars(c,dst,8,&src,1,&e);
  if (U_FAILURE(e)) write32(0);
  else if (n==0) write32(257);
  else if (n==1) write32((uint8_t)dst[0]+1);
  else {fprintf(stderr,"unexpected %d-byte output for U+%04X\n",n,r);return 1;}
 }
 ucnv_close(c);
 return ferror(stdout)?1:0;
}
