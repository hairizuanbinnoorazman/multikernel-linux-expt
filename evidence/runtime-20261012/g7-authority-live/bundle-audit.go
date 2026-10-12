package main
import("fmt";"os";"syscall";"github.com/hairizuan/multikernel-linux-expt/runtime/agent")
func main(){ p:="/bundle/config.json"; f,e:=os.Stat(p);if e!=nil{fmt.Printf("BUNDLE_STAT_ERROR %v\n",e)}else{fmt.Printf("BUNDLE_STAT mode=%v size=%d stat=%+v\n",f.Mode(),f.Size(),f.Sys().(*syscall.Stat_t))}; _,r,e:=agent.LoadBundle("/bundle");fmt.Printf("BUNDLE_AUDIT root=%s error=%v\n",r,e) }
