package com.example.tako;

/**
 * Core library class for BOM E2E test scenario.
 * Provides foundational functionality that other libraries depend on.
 */
public class CoreLib {
    
    private static final String VERSION = "1.0.0";
    
    /**
     * Returns the version of the core library.
     */
    public static String getVersion() {
        return VERSION;
    }
    
    /**
     * Core functionality that dependent libraries will use.
     */
    public String getCoreMessage() {
        return "Hello from CoreLib v" + VERSION;
    }
    
    /**
     * A method that might change in future versions to test dependency updates.
     */
    public int calculateValue(int input) {
        return input * 2;
    }
}