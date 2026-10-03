import ghidra.app.script.GhidraScript;
import ghidra.app.decompiler.*;
import ghidra.app.cmd.disassemble.DisassembleCommand;
import ghidra.app.cmd.function.CreateFunctionCmd;
import ghidra.program.model.address.*;
import ghidra.program.model.listing.*;
import java.io.*;

// Args: output file, then hex addresses. Creates missing functions first.
public class DecompileAt extends GhidraScript {
    @Override
    public void run() throws Exception {
        String[] args = getScriptArgs();
        DecompInterface d = new DecompInterface();
        // Very large functions (the packet dispatcher) exceed the default
        // output and instruction limits rather than the time limit.
        DecompileOptions o = new DecompileOptions();
        o.setMaxPayloadMBytes(1024);
        o.setMaxInstructions(1000000);
        d.setOptions(o);
        d.openProgram(currentProgram);
        try (PrintWriter w = new PrintWriter(new BufferedWriter(new FileWriter(args[0])))) {
            for (int i = 1; i < args.length; i++) {
                Address a = toAddr(Long.parseLong(args[i], 16));
                Function f = getFunctionAt(a);
                if (f == null) {
                    // An instruction decoded from padding just before the
                    // entry can cover it; clear it so the entry disassembles.
                    CodeUnit cu = currentProgram.getListing().getCodeUnitContaining(a);
                    if (cu != null && cu.getMinAddress().compareTo(a) < 0) {
                        clearListing(cu.getMinAddress(), cu.getMaxAddress());
                    }
                    new DisassembleCommand(a, null, true).applyTo(currentProgram, monitor);
                    new CreateFunctionCmd(a).applyTo(currentProgram, monitor);
                    f = getFunctionAt(a);
                }
                if (f == null) {
                    // Auto-analysis sometimes folds a function into the body
                    // of the one before it. Split them: drop the enclosing
                    // function, create this one, then recreate the enclosing
                    // one so its body stops here.
                    Function outer = getFunctionContaining(a);
                    if (outer != null) {
                        Address outerEntry = outer.getEntryPoint();
                        removeFunction(outer);
                        new CreateFunctionCmd(a).applyTo(currentProgram, monitor);
                        new CreateFunctionCmd(outerEntry).applyTo(currentProgram, monitor);
                        f = getFunctionAt(a);
                    }
                }
                if (f == null) { w.println("// no function at " + a); continue; }
                w.println("// Function: " + f.getName() + " @ " + f.getEntryPoint());
                String env = System.getenv("DECOMPILE_TIMEOUT"); // seconds
                int timeout = env == null ? 3000 : Integer.parseInt(env);
                DecompileResults r = d.decompileFunction(f, timeout, monitor);
                if (r != null && r.decompileCompleted()) {
                    w.println(r.getDecompiledFunction().getC());
                } else {
                    w.println("// decompile failed: " + (r == null ? "no result" : r.getErrorMessage()));
                }
                w.flush();
            }
        }
    }
}
