import ghidra.app.script.GhidraScript;
import ghidra.app.cmd.disassemble.DisassembleCommand;
import ghidra.app.cmd.function.CreateFunctionCmd;
import ghidra.program.model.address.*;
import ghidra.program.model.mem.*;
import ghidra.program.model.listing.*;

// Creates functions at undisassembled, 4-byte-aligned "push ebp; mov ebp,esp"
// prologues in CODE, repeating until none are added. Delphi code reached only
// through VMTs and form callbacks is otherwise missed by auto-analysis.
public class CreateMissing extends GhidraScript {
    @Override
    public void run() throws Exception {
        MemoryBlock code = currentProgram.getMemory().getBlock("CODE");
        long start = code.getStart().getOffset();
        byte[] b = new byte[(int) code.getSize()];
        code.getBytes(code.getStart(), b);
        for (int pass = 0; pass < 6; pass++) {
            int added = 0;
            Listing l = currentProgram.getListing();
            for (int i = 0; i + 3 <= b.length; i++) {
                if (((start + i) & 3) != 0 || (b[i] & 0xff) != 0x55 || (b[i+1] & 0xff) != 0x8B || (b[i+2] & 0xff) != 0xEC) continue;
                Address a = code.getStart().add(i);
                if (l.getInstructionContaining(a) != null || getFunctionAt(a) != null) continue;
                new DisassembleCommand(a, null, true).applyTo(currentProgram, monitor);
                if (new CreateFunctionCmd(a).applyTo(currentProgram, monitor)) added++;
            }
            println("pass " + pass + ": created " + added);
            if (added == 0) break;
        }
    }
}
