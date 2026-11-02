/* Query records: length (1..4 for decode, 0 for Unicode encode), 4 payload bytes. */
#include <stdint.h>
#include <stdio.h>
#include <unicode/ucnv.h>
#include <unicode/ucnv_err.h>
#include <unicode/ustring.h>
#include <unicode/utf16.h>

int main(int argc,char **argv){
 if(argc!=3)return 2;UErrorCode e=U_ZERO_ERROR;UConverter*c=ucnv_openPackage(argv[1],argv[2],&e);if(U_FAILURE(e)){fprintf(stderr,"open: %s\n",u_errorName(e));return 1;}
 ucnv_setToUCallBack(c,UCNV_TO_U_CALLBACK_STOP,NULL,NULL,NULL,&e);ucnv_setFromUCallBack(c,UCNV_FROM_U_CALLBACK_STOP,NULL,NULL,NULL,&e);if(U_FAILURE(e))return 1;
 unsigned char req[5];while(fread(req,1,5,stdin)==5){unsigned char out[9]={0};char encoded[8]={0};int32_t n=0;e=U_ZERO_ERROR;ucnv_reset(c);
  if(req[0]){UChar unicode[8];int len=ucnv_toUChars(c,unicode,8,(const char*)req+1,req[0],&e);if(U_SUCCESS(e))u_strToUTF8(encoded,8,&n,unicode,len,&e);}
  else{uint32_t r=(uint32_t)req[1]<<24|(uint32_t)req[2]<<16|(uint32_t)req[3]<<8|req[4];UChar unicode[2];int len=0;U16_APPEND_UNSAFE(unicode,len,r);n=ucnv_fromUChars(c,encoded,8,unicode,len,&e);}
  if(U_SUCCESS(e)&&n>=0&&n<=8){out[0]=(unsigned char)(n+1);for(int i=0;i<n;i++)out[i+1]=(unsigned char)encoded[i];}
  if(fwrite(out,1,9,stdout)!=9)return 1;
 }
 ucnv_close(c);return ferror(stdin)||ferror(stdout)?1:0;
}
