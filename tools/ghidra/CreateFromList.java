import ghidra.app.script.GhidraScript;
import ghidra.app.cmd.disassemble.DisassembleCommand;
import ghidra.app.cmd.function.CreateFunctionCmd;
import ghidra.program.model.address.*;
import java.nio.file.*;

// Creates functions at every hex address listed in a file (one per line),
// e.g. VMT slot targets and published methods from delphi_meta.py. Delphi
// methods reached only through VMTs are otherwise missed by auto-analysis.
public class CreateFromList extends GhidraScript {
    @Override
    public void run() throws Exception {
        int added = 0;
        for (String line : Files.readAllLines(Paths.get(getScriptArgs()[0]))) {
            line = line.trim();
            if (line.isEmpty()) continue;
            Address a = toAddr(Long.parseLong(line, 16));
            if (getFunctionAt(a) != null) continue;
            new DisassembleCommand(a, null, true).applyTo(currentProgram, monitor);
            if (new CreateFunctionCmd(a).applyTo(currentProgram, monitor)) added++;
        }
        println("created " + added);
    }
}
