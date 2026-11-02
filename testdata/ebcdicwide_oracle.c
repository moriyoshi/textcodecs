/* Query: mode byte, followed by 8 bytes. 1..4 decode; 0/5 encode one/two scalars. */
#include <stdint.h>
#include <stdio.h>
#include <unicode/ucnv.h>
#include <unicode/ucnv_err.h>
#include <unicode/ustring.h>
#include <unicode/utf16.h>
static uint32_t get32(const unsigned char *b){return (uint32_t)b[0]<<24|(uint32_t)b[1]<<16|(uint32_t)b[2]<<8|b[3];}
int main(int argc,char**argv){if(argc!=3)return 2;UErrorCode e=U_ZERO_ERROR;UConverter*c=ucnv_openPackage(argv[1],argv[2],&e);if(U_FAILURE(e)){fprintf(stderr,"open: %s\n",u_errorName(e));return 1;}ucnv_setToUCallBack(c,UCNV_TO_U_CALLBACK_STOP,NULL,NULL,NULL,&e);ucnv_setFromUCallBack(c,UCNV_FROM_U_CALLBACK_STOP,NULL,NULL,NULL,&e);if(U_FAILURE(e))return 1;
 unsigned char req[9];while(fread(req,1,9,stdin)==9){unsigned char out[17]={0};char result[16]={0};int32_t n=0;e=U_ZERO_ERROR;ucnv_reset(c);
  if(req[0]>=1&&req[0]<=4){UChar u[16];int len=ucnv_toUChars(c,u,16,(const char*)req+1,req[0],&e);if(U_SUCCESS(e))u_strToUTF8(result,16,&n,u,len,&e);}else{UChar u[4];int len=0;U16_APPEND_UNSAFE(u,len,get32(req+1));if(req[0]==5)U16_APPEND_UNSAFE(u,len,get32(req+5));n=ucnv_fromUChars(c,result,16,u,len,&e);}
  if(U_SUCCESS(e)&&n>=0&&n<=16){out[0]=(unsigned char)(n+1);for(int i=0;i<n;i++)out[i+1]=(unsigned char)result[i];}if(fwrite(out,1,17,stdout)!=17)return 1;
 }
 ucnv_close(c);return ferror(stdin)||ferror(stdout)?1:0;
}
