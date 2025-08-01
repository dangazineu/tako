package com.example.tako;

/**
 * Library B class that depends on CoreLib.
 * Demonstrates dependency updates in BOM E2E test scenario.
 */
public class LibB {
    
    private static final String VERSION = "1.0.0";
    private final CoreLib coreLib;
    
    public LibB() {
        this.coreLib = new CoreLib();
    }
    
    /**
     * Returns the version of Library B.
     */
    public static String getVersion() {
        return VERSION;
    }
    
    /**
     * Functionality that uses CoreLib.
     */
    public String getLibBMessage() {
        return "LibB v" + VERSION + " extends: " + coreLib.getCoreMessage();
    }
    
    /**
     * Different enhancement functionality building on CoreLib.
     */
    public int advancedCalculation(int input) {
        int coreResult = coreLib.calculateValue(input);
        return coreResult * 3; // LibB different enhancement
    }
    
    /**
     * Gets the version of the underlying core library.
     */
    public String getCoreLibVersion() {
        return CoreLib.getVersion();
    }
    
    /**
     * Utility method specific to LibB.
     */
    public boolean isPositive(int value) {
        return value > 0;
    }
}