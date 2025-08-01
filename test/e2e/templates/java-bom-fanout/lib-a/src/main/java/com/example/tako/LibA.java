package com.example.tako;

/**
 * Library A class that depends on CoreLib.
 * Demonstrates dependency updates in BOM E2E test scenario.
 */
public class LibA {
    
    private static final String VERSION = "1.0.0";
    private final CoreLib coreLib;
    
    public LibA() {
        this.coreLib = new CoreLib();
    }
    
    /**
     * Returns the version of Library A.
     */
    public static String getVersion() {
        return VERSION;
    }
    
    /**
     * Functionality that uses CoreLib.
     */
    public String getLibAMessage() {
        return "LibA v" + VERSION + " uses: " + coreLib.getCoreMessage();
    }
    
    /**
     * Enhanced functionality building on CoreLib.
     */
    public int enhancedCalculation(int input) {
        int coreResult = coreLib.calculateValue(input);
        return coreResult + 10; // LibA enhancement
    }
    
    /**
     * Gets the version of the underlying core library.
     */
    public String getCoreLibVersion() {
        return CoreLib.getVersion();
    }
}